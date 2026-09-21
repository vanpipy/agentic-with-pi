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

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
	tea "charm.land/bubbletea/v2"

	"github.com/vanpiyp/awp/internal/client-sdk"
	"github.com/vanpiyp/awp/internal/log"
	"github.com/vanpiyp/awp/internal/storage"
	"github.com/vanpiyp/awp/internal/transport"
	"golang.org/x/term"
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
	spinner      spinner.Model
	help         help.Model
	keys         keyBindings
	showHelp     bool
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
		spinner:      newSpinner(),
		help:         help.New(),
		keys:         defaultKeys(),
		showHelp:     false,
	}

	p := tea.NewProgram(m)
	program = p
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}

	return conn.Close()
}

func (m *Model) Init() tea.Cmd {
	m.spinner = newSpinner()
	return func() tea.Msg { return m.spinner.Tick() }
}

func newSpinner() spinner.Model {
	s := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	s.Style = statusSpin
	return s
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()

	case tea.MouseWheelMsg:
		cmd, _ := m.chat.Update(msg)
		cmds = append(cmds, cmd)

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
		case "?":
			m.showHelp = !m.showHelp
		case "esc":
			if m.showHelp {
				m.showHelp = false
				break
			}
			if m.state == stateStreaming {
				m.cancel()
				break
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
		m.autocomplete.hide()

		if strings.HasPrefix(text, "/") {
			shouldQuit, cmd := m.executeCommand(text)
			m.layout()
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
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

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
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

	header := renderHeader(m.width, m.statusRender(), sessionLabel(m.session))

	inputBox := m.input.View()

	m.help.ShowAll = m.showHelp
	var footer string
	if m.showHelp {
		footer = ""
	} else {
		footer = m.help.ShortHelpView(m.keys.ShortHelp())
	}

	if m.showHelp {
		helpText := systemPrefix.Render(" help") + "\n" +
			m.help.FullHelpView(m.keys.FullHelp())
		lines := []string{header, "", helpText, "", inputBox}
		v := tea.NewView(strings.Join(lines, "\n"))
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		return v
	}

	body := m.chat.View()
	lines := []string{header, "", body, "", inputBox, "", footer}
	if m.autocomplete.visible {
		lines = append(lines, "", m.autocomplete.View())
	}
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m *Model) layout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	footerLines := 1
	if m.showHelp {
		footerLines = len(strings.Split(m.help.FullHelpView(m.keys.FullHelp()), "\n"))
	}
	staticLines := 1 + 1 + 1 + m.input.Height() + 1 + footerLines
	popupLines := 0
	if m.autocomplete.visible {
		popupLines = strings.Count(m.autocomplete.View(), "\n") + 1 + 1
	}
	reservedLines := staticLines + popupLines
	bodyHeight := m.height - reservedLines
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	if !m.showHelp {
		m.chat.SetSize(m.width-4, bodyHeight)
	}
	m.help.SetWidth(m.width)
	m.input.SetWidth(m.width - 2)
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

func NewModelForTest() *Model {
	m := &Model{
		state:        stateReady,
		chat:         newChatModel(),
		input:        newInputModel(),
		autocomplete: newAutocompleteModel(),
		spinner:      newSpinner(),
		help:         help.New(),
		keys:         defaultKeys(),
		width:        80,
		height:       40,
	}
	return m
}

func (m *Model) ShowHelpForTest() bool { return m.showHelp }

func (m *Model) AutocompleteVisibleForTest() bool { return m.autocomplete.visible }

func (m *Model) StateForTest() State { return m.state }

func (m *Model) SetStateForTest(s State) { m.state = s }

func (m *Model) InputValueForTest() string { return m.input.Value() }

func (m *Model) AutocompleteViewForTest() string { return m.autocomplete.View() }

func (m *Model) shutdown() {
	if m.conn != nil {
		m.conn.Close()
	}
}

func (m *Model) cancel() {
	if m.conn != nil {
		if err := m.conn.Cancel(context.Background()); err != nil {
			m.chat.appendError("cancel: " + err.Error())
		}
	}
	m.chat.appendSystem(systemPrefix.Render(" cancelled by user"))
	m.state = stateReady
	m.events = nil
}

func (m *Model) submit(text string) tea.Cmd {
	m.state = stateStreaming
	m.chat.submit(text)
	m.chat.GotoBottom()
	return m.startStream(text)
}

type promptDoneMsg struct{}

var program *tea.Program



func shortHelpBindings() []key.Binding {
	return defaultKeys().ShortHelp()
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
		return m.spinner.View() + " " + m.state.String()
	default:
		return m.state.String()
	}
}

func renderHeader(width int, status, session string) string {
	if width <= 0 {
		return ""
	}
	left := headerBrand.Render("awp") +
		headerSeparator.Render(" │ ") +
		status
	right := headerSession.Render(session)
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + headerTrack.Render(strings.Repeat(" ", gap)) + right
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
