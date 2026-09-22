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
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
	state         State
	err           error
	width         int
	height        int
	conn          *client_sdk.Client
	serverPID     int
	ownServer     bool
	session       string
	lastPrompt    string
	lastPromptNum int
	chat          *chatModel
	input         *inputModel
	autocomplete  *autocompleteModel
	picker        *sessionPickerModel
	events        <-chan client_sdk.Event
	lastKind      string
	spinner       spinner.Model
	help          help.Model
	keys          keyBindings
}

type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }

type promptSubmittedMsg struct {
	text string
}

type streamEventMsg struct {
	ev   client_sdk.Event
	err  error
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

	clientPID := os.Getpid()
	socket := storage.ClientSocketPath(clientPID)
	pidFile := storage.ServerPidPath(clientPID)

	ownServer := true
	if pid, err := transport.ReadServerPID(pidFile); err == nil {
		if transport.IsAlive(pid) {
			ownServer = false
			fmt.Fprintf(os.Stderr, "reusing server pid=%d from %s\n", pid, pidFile)
		} else {
			fmt.Fprintf(os.Stderr, "stale pidfile %s -> pid %d gone, cleaning\n", pidFile, pid)
			transport.RemoveServerPID(pidFile)
		}
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "warning: read pidfile %s: %v\n", pidFile, err)
	}

	if ownServer {
		fmt.Fprintf(os.Stderr, "no server on %s, spawning...\n", socket)
		serverPID, err := spawnServer(socket)
		if err != nil {
			return fmt.Errorf("spawn server: %w", err)
		}
		if err := transport.WriteServerPID(pidFile, serverPID); err != nil {
			_ = syscall.Kill(serverPID, syscall.SIGTERM)
			return fmt.Errorf("write pidfile: %w", err)
		}
		if err := waitForServer(socket, 5*time.Second); err != nil {
			_ = syscall.Kill(serverPID, syscall.SIGTERM)
			transport.RemoveServerPID(pidFile)
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
		serverPID:    ownServerPID(socket, pidFile),
		ownServer:    ownServer,
		chat:         newChatModel(),
		input:        newInputModel(),
		autocomplete: newAutocompleteModel(),
		picker:       newSessionPickerModel(),
		spinner:      newSpinner(),
		keys:         defaultKeys(),
	}

	p := tea.NewProgram(m)
	program = p
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}

	return conn.Close()
}

func ownServerPID(socket, pidFile string) int {
	pid, err := transport.ReadServerPID(pidFile)
	if err != nil {
		return 0
	}
	if !transport.IsAlive(pid) {
		_ = transport.RemoveServerPID(pidFile)
		return 0
	}
	return pid
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

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			if m.input.Value() == "" {
				m.shutdown()
				return m, tea.Quit
			}
			cmds = append(cmds, m.input.Update(msg))
		case "ctrl+d":
			if m.input.Value() == "" {
				m.shutdown()
				return m, tea.Quit
			}
			cmds = append(cmds, m.input.Update(msg))
		case "ctrl+e":
			m.chat.ToggleCollapseAtViewportTop()
		case "esc":
			if m.autocomplete.visible {
				m.autocomplete.hide()
				m.input.Reset()
				break
			}
			if m.picker.visible {
				m.picker.hide()
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
			if m.picker.visible {
				selected := m.picker.current()
				m.picker.hide()
				m.input.Reset()
				if selected != "" {
					cmds = append(cmds, m.startResume(selected))
				}
				break
			}
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
			m.lastPrompt = text
			m.lastPromptNum = m.chat.promptNum
			m.state = stateStreaming
			cmds = append(cmds, m.startStream(text))
		case "up":
			if m.picker.visible {
				m.picker.prev()
				break
			}
			if m.autocomplete.visible {
				m.autocomplete.prev()
				break
			}
			m.chat.ScrollUp(1)
		case "down":
			if m.picker.visible {
				m.picker.next()
				break
			}
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
		wasAtBottom := m.chat.AtBottomExisting()
		handleServerEvent(m.chat, &m.session, msg.ev)
		if wasAtBottom {
			m.chat.GotoBottom()
		}
		m.lastKind = msg.ev.Kind
		if msg.done {
			m.state = stateReady
			m.events = nil
		} else if msg.ev.Kind == "final_answer" {
			m.state = stateReady
		} else if msg.ev.Kind == "error" {
			m.state = stateError
		}
		if m.events != nil {
			cmds = append(cmds, m.readNextEvent())
		}

	case errMsg:
		m.err = msg.err
		m.state = stateError
		m.chat.appendError(msg.err.Error())

	case sessionPickerMsg:
		if len(msg.items) == 0 {
			m.chat.appendSystem(systemPrefix.Render(" no saved sessions"))
			break
		}
		m.picker.Show(msg.items)

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

	header := renderHeader(m.width, m.statusRender(), m.lastPrompt, m.session)

	inputBox := m.input.View()

	footer := m.help.ShortHelpView(m.keys.ShortHelp())
	footerLines := 1

	popup := ""
	if m.picker.visible {
		popup = m.picker.View()
	} else if m.autocomplete.visible {
		popup = m.autocomplete.View()
	}
	popupLineCount := 0
	if popup != "" {
		popupLineCount = strings.Count(popup, "\n") + 1
	}

	body := m.chat.View()
	bodyLines := strings.Split(body, "\n")

	fixedLines := 1 + 1 + 1 + m.input.Height() + 1 + footerLines
	availableForBody := m.height - fixedLines - popupLineCount
	if availableForBody < 1 {
		availableForBody = 1
	}
	if len(bodyLines) > availableForBody {
		bodyLines = bodyLines[:availableForBody]
	}
	if len(bodyLines) < availableForBody {
		padding := make([]string, availableForBody-len(bodyLines))
		bodyLines = append(bodyLines, padding...)
	}

	lines := []string{header, "", strings.Join(bodyLines, "\n")}
	if popup != "" {
		lines = append(lines, popup)
	}
	lines = append(lines, "", inputBox, "", footer)

	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}

func (m *Model) layout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	footerLines := 1
	staticLines := 1 + 1 + 1 + m.input.Height() + 1 + footerLines
	bodyHeight := m.height - staticLines
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	m.chat.SetSize(m.width-4, bodyHeight)
	m.help.SetWidth(m.width)
	m.input.SetWidth(m.width - 2)
}

func (m *Model) startStream(text string) tea.Cmd {
	ctx := context.Background()
	conn := m.conn

	events, err := conn.PromptWithSessionID(ctx, text, m.session)
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
		picker:       newSessionPickerModel(),
		spinner:      newSpinner(),
		help:         help.New(),
		keys:         defaultKeys(),
		width:        80,
		height:       40,
	}
	return m
}

func (m *Model) AutocompleteVisibleForTest() bool { return m.autocomplete.visible }

func (m *Model) PickerVisibleForTest() bool { return m.picker.visible }

func (m *Model) PickerViewForTest() string { return m.picker.View() }

func (m *Model) StateForTest() State { return m.state }

func (m *Model) SetStateForTest(s State) { m.state = s }

func (m *Model) InputValueForTest() string { return m.input.Value() }

func (m *Model) AutocompleteViewForTest() string { return m.autocomplete.View() }

func (m *Model) shutdown() {
	if m.conn != nil {
		m.conn.Close()
	}
	if m.ownServer && m.serverPID > 0 {
		_ = syscall.Kill(m.serverPID, syscall.SIGTERM)
		deadline := time.Now().Add(2 * time.Second)
		for transport.IsAlive(m.serverPID) && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
		if transport.IsAlive(m.serverPID) {
			_ = syscall.Kill(m.serverPID, syscall.SIGKILL)
		}
		transport.RemoveServerPID(storage.ServerPidPath(os.Getpid()))
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

func sessionLabel(id string, maxWidth int) string {
	if id == "" {
		return "(none)"
	}
	if maxWidth <= 0 || len(id) <= maxWidth {
		return id
	}
	if maxWidth <= 1 {
		return id[:maxWidth]
	}
	return id[:maxWidth-1] + "…"
}

func SessionLabelForTest(id string, maxWidth int) string {
	return sessionLabel(id, maxWidth)
}

func RenderHeaderForTest(width int, status, sessionID string) string {
	return renderHeader(width, status, "", sessionID)
}

func RenderHeaderWithPromptForTest(width int, status, lastPrompt, sessionID string) string {
	return renderHeader(width, status, lastPrompt, sessionID)
}

func (m *Model) statusRender() string {
	switch m.state {
	case stateStreaming:
		return m.spinner.View() + " " + m.state.String()
	case stateError:
		return statusErr.Render("● ") + m.state.String()
	default:
		return statusOK.Render("● ") + m.state.String()
	}
}

func renderHeader(width int, status, lastPrompt, sessionID string) string {
	if width <= 0 {
		return ""
	}
	statusColumnWidth := statusFixedWidth()
	statusPart := headerStatus.Width(statusColumnWidth).Render(status)
	sessionIDContentWidth := headerSessionIDFixedWidth
	sessionColumnWidth := sessionIDContentWidth + headerSessionID.GetHorizontalPadding()
	if width < statusColumnWidth+sessionColumnWidth+4 {
		sessionColumnWidth = width - statusColumnWidth
		if sessionColumnWidth < 4 {
			sessionColumnWidth = 4
		}
		sessionIDContentWidth = sessionColumnWidth - headerSessionID.GetHorizontalPadding()
		if sessionIDContentWidth < 4 {
			sessionIDContentWidth = 4
		}
	}
	sessionText := sessionLabel(sessionID, sessionIDContentWidth)
	sessionPart := headerSessionID.Width(sessionColumnWidth).Render(sessionText)
	promptColumnWidth := width - statusColumnWidth - sessionColumnWidth
	if promptColumnWidth < 4 {
		promptColumnWidth = 4
	}
	promptContentWidth := promptColumnWidth - headerPrompt.GetHorizontalPadding()
	if promptContentWidth < 1 {
		promptContentWidth = 1
	}
	promptText := truncateWithEllipsis(lastPrompt, promptContentWidth)
	promptPart := headerPrompt.Width(promptColumnWidth).Render(promptText)
	return lipgloss.JoinHorizontal(lipgloss.Top, statusPart, promptPart, sessionPart)
}

func statusFixedWidth() int {
	const widest = "● streaming"
	return lipgloss.Width(widest) + headerStatus.GetHorizontalPadding()
}

func truncateWithEllipsis(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	if maxWidth <= 1 {
		return "…"
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > maxWidth {
		runes = runes[:len(runes)-1]
	}
	if len(runes) == 0 {
		return "…"
	}
	return string(runes) + "…"
}

func spawnServer(socket string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}
	cmd := exec.Command(exe, "serve", "--socket", socket)
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
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
