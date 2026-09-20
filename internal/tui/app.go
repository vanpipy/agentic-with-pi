package tui

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"

	"github.com/vanpiyp/awp/internal/client-sdk"
	"github.com/vanpiyp/awp/internal/log"
	"github.com/vanpiyp/awp/internal/storage"
	"github.com/vanpiyp/awp/internal/transport"
)

type State int

const (
	stateReady State = iota
	stateStreaming
	stateError
)

func (s State) String() string {
	switch s {
	case stateReady:
		return "ready"
	case stateStreaming:
		return "streaming"
	case stateError:
		return "error"
	}
	return "unknown"
}

type Model struct {
	state        State
	err          error
	width        int
	height       int
	conn         *client_sdk.Client
	session      string
	chat         *chatModel
	input        *inputModel
	autocomplete *autocompleteModel
	events       <-chan client_sdk.Event
	lastKind     string
	spinnerFrame int
}

type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }

type promptSubmittedMsg struct {
	text string
}

type streamEventMsg struct {
	ev  client_sdk.Event
	err error
	done bool
}

func Run() error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("awp tui requires a terminal; use 'awp connect' instead")
	}

	logCfg := log.DefaultConfig()
	logCfg.Level = slog.LevelInfo
	if err := log.Setup(logCfg); err != nil {
		fmt.Fprintln(os.Stderr, "log setup:", err)
	}

	socket := storage.SocketPath()
	if !transport.IsRunning(socket) {
		fmt.Fprintln(os.Stderr, "server not running, spawning...")
		if err := spawnServer(); err != nil {
			return fmt.Errorf("spawn server: %w", err)
		}
		if err := waitForServer(socket, 5*time.Second); err != nil {
			return fmt.Errorf("wait server: %w", err)
		}
	}

	conn, err := client_sdk.Dial(socket)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	if err := conn.Ping(); err != nil {
		conn.Close()
		return fmt.Errorf("ping: %w", err)
	}

	m := &Model{
		state:        stateReady,
		conn:         conn,
		chat:         newChatModel(),
		input:        newInputModel(),
		autocomplete: newAutocompleteModel(),
	}

	p := tea.NewProgram(m)
	program = p
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}

	return conn.Close()
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		textinputBlinkCmd(),
		m.spinnerTickCmd(),
	)
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (m *Model) spinnerTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

type spinnerTickMsg struct{}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()

	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.chat.ScrollUp(3)
		case tea.MouseWheelDown:
			m.chat.ScrollDown(3)
		}

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			m.shutdown()
			return m, tea.Quit
		case "ctrl+d":
			if m.input.Value() == "" {
				m.shutdown()
				return m, tea.Quit
			}
			cmds = append(cmds, m.input.Update(msg))
		case "tab":
			if m.autocomplete.visible {
				m.acceptAutocomplete()
				break
			}
			cmds = append(cmds, m.input.Update(msg))
		case "enter":
			if m.state == stateStreaming {
				break
			}
			text := m.input.Value()
			if strings.TrimSpace(text) == "" {
				break
			}
			m.input.Reset()

			if strings.HasPrefix(text, "/") {
				shouldQuit := m.executeCommand(text)
				m.layout()
				if shouldQuit {
					return m, tea.Quit
				}
				return m, tea.Batch(cmds...)
			}

			m.chat.submit(text)
			m.chat.GotoBottom()
			m.state = stateStreaming
			cmds = append(cmds, m.startStream(text))
		case "up":
			if m.autocomplete.visible {
				m.autocomplete.prev()
				break
			}
			m.chat.ScrollUp(1)
		case "down":
			if m.autocomplete.visible {
				m.autocomplete.next()
				break
			}
			m.chat.ScrollDown(1)
		case "pgup":
			m.chat.HalfPageUp()
		case "pgdown":
			m.chat.HalfPageDown()
		case "ctrl+up":
			m.chat.JumpToPrompt(-1)
		case "ctrl+down":
			m.chat.JumpToPrompt(1)
		default:
			cmds = append(cmds, m.input.Update(msg))
		}
		m.autocomplete.setQuery(m.input.Value())

	case streamEventMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = stateError
			m.chat.appendError(msg.err.Error())
			break
		}
		wasAtBottom := m.chat.AtBottom()
		handleServerEvent(m.chat, &m.session, msg.ev)
		if wasAtBottom {
			m.chat.GotoBottom()
		}
		m.lastKind = msg.ev.Kind
		if msg.done {
			m.state = stateReady
			m.events = nil
		} else if msg.ev.Kind == "final_answer" || msg.ev.Kind == "error" {
			m.state = stateReady
		}
		if m.events != nil {
			cmds = append(cmds, m.readNextEvent())
		}

	case errMsg:
		m.err = msg.err
		m.state = stateError
		m.chat.appendError(msg.err.Error())

	case spinnerTickMsg:
		m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
		if m.state == stateStreaming || m.lastKind != "" {
			cmds = append(cmds, m.spinnerTickCmd())
		}

	case promptDoneMsg:
		m.state = stateReady
	}

	m.layout()
	return m, tea.Batch(cmds...)
}

func (m *Model) View() tea.View {
	if m.width == 0 {
		return tea.NewView("initializing...")
	}

	status := m.statusRender()

	header := fmt.Sprintf(" awp  [%s]  session: %s ", status, sessionLabel(m.session))

	body := m.chat.View()

	inputBox := m.input.View()

	footer := fmt.Sprintf(
		" ● ready  ctrl+c: quit  /quit: quit  ctrl+d: quit/eof  tab: complete  ↑↓: scroll",
	)

	lines := []string{header, "", body, "", inputBox, "", footer}
	if m.autocomplete.visible {
		lines = append(lines, "", m.autocomplete.View())
	}
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func statusOK(status string) string {
	return "● " + status
}

func (m *Model) layout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	bodyHeight := m.height - 5
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	m.chat.SetSize(m.width-4, bodyHeight)
}

func (m *Model) startStream(text string) tea.Cmd {
	ctx := context.Background()
	conn := m.conn

	events, err := conn.Prompt(ctx, text)
	if err != nil {
		return func() tea.Msg { return errMsg{err} }
	}
	m.events = events
	return m.readNextEvent()
}

func (m *Model) readNextEvent() tea.Cmd {
	if m.events == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-m.events
		if !ok {
			return streamEventMsg{done: true}
		}
		return streamEventMsg{ev: ev}
	}
}

func (m *Model) shutdown() {
	if m.conn != nil {
		m.conn.Close()
	}
}

func (m *Model) submit(text string) {
	m.state = stateStreaming
	cmds := m.startStream(text)
	_ = cmds
}

func (m *Model) resumeSession(sessionID string) {
	events, err := m.conn.Resume(context.Background(), sessionID)
	if err != nil {
		m.chat.appendError("resume: " + err.Error())
		return
	}
	m.session = sessionID
	m.chat.reset()
	for ev := range events {
		handleServerEvent(m.chat, &m.session, ev)
	}
}

type promptDoneMsg struct{}

var program *tea.Program

func textinputBlinkCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*500, func(t time.Time) tea.Msg {
		return nil
	})
}

func sessionLabel(id string) string {
	if id == "" {
		return "(none)"
	}
	if len(id) > 12 {
		return id[:8] + "..."
	}
	return id
}

func (m *Model) statusRender() string {
	switch m.state {
	case stateStreaming:
		frame := spinnerFrames[m.spinnerFrame]
		return frame + " " + m.state.String()
	case stateError:
		return "● " + m.state.String()
	default:
		return "● " + m.state.String()
	}
}

func spawnServer() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "serve")
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}

func waitForServer(socketPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if transport.IsRunning(socketPath) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("server did not start within %v", timeout)
}
