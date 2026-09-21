package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/vanpiyp/awp/internal/agent/tools"
	client_sdk "github.com/vanpiyp/awp/internal/client-sdk"
)

type commandSpec struct {
	Name        string
	Description string
	Category    string
	HasArg      bool
	Usage       string
	Run         func(m *Model, arg string) (quit bool, cmd tea.Cmd)
}

var registry = []commandSpec{
	{
		Name:        "quit",
		Description: "exit the TUI",
		Category:    "exit",
		Run: func(m *Model, _ string) (bool, tea.Cmd) {
			return true, nil
		},
	},
	{
		Name:        "help",
		Description: "show available slash commands",
		Category:    "help",
		Run: func(m *Model, _ string) (bool, tea.Cmd) {
			m.showHelp = true
			return false, nil
		},
	},
	{
		Name:        "new",
		Description: "clear chat history, start fresh turn",
		Category:    "session",
		Run: func(m *Model, _ string) (bool, tea.Cmd) {
			m.chat.reset()
			return false, nil
		},
	},
	{
		Name:        "tools",
		Description: "list available tools",
		Category:    "help",
		Run: func(m *Model, _ string) (bool, tea.Cmd) {
			m.cmdTools()
			return false, nil
		},
	},
	{
		Name:        "resume",
		Description: "resume a previous session by id",
		Category:    "session",
		HasArg:      true,
		Usage:       "/resume <session_id>",
		Run: func(m *Model, arg string) (bool, tea.Cmd) {
			if strings.TrimSpace(arg) == "" {
				m.chat.appendSystem(helpFooter.Render(" usage: /resume <session_id>"))
				return false, nil
			}
			return false, m.startResume(arg)
		},
	},
}

func (m *Model) startResume(sessionID string) tea.Cmd {
	events, err := m.conn.Resume(context.Background(), sessionID)
	if err != nil {
		m.chat.appendError("resume: " + err.Error())
		return nil
	}
	m.session = sessionID
	m.chat.reset()
	m.state = stateStreaming
	m.events = events
	return m.readNextEvent()
}

func findCommand(name string) (commandSpec, bool) {
	for _, c := range registry {
		if c.Name == name {
			return c, true
		}
	}
	return commandSpec{}, false
}

func allCommandSpecs() []commandSpec {
	return registry
}

func parseCommand(text string) (parsedCommand, bool) {
	if !strings.HasPrefix(text, "/") {
		return parsedCommand{}, false
	}
	body := strings.TrimPrefix(text, "/")
	parts := strings.SplitN(body, " ", 2)
	cmd := parsedCommand{name: parts[0]}
	if len(parts) > 1 {
		cmd.arg = strings.TrimSpace(parts[1])
		cmd.rest = parts[1]
	}
	return cmd, true
}

type parsedCommand struct {
	name string
	arg  string
	rest string
}

func (m *Model) executeCommand(text string) (quit bool, cmd tea.Cmd) {
	parsed, ok := parseCommand(text)
	if !ok {
		return false, nil
	}

	spec, found := findCommand(parsed.name)
	if !found {
		m.chat.appendSystem(errorPrefix.Render(" unknown command: /" + parsed.name) +
			helpFooter.Render("\n  type /help for available commands"))
		return false, nil
	}
	return spec.Run(m, parsed.arg)
}

func (m *Model) cmdHelp() {
	m.showHelp = true
}

func (m *Model) cmdTools() {
	all := tools.All("")
	lines := []string{systemPrefix.Render(" available tools:"), ""}
	for _, t := range all {
		lines = append(lines, "  "+toolName.Render(padName(t.Name))+helpFooter.Render(t.Description))
	}
	m.chat.appendSystem(strings.Join(lines, "\n"))
}

func padName(name string) string {
	const pad = 10
	if len(name) >= pad {
		return name + " "
	}
	return name + strings.Repeat(" ", pad-len(name))
}



type ParsedCommand struct {
	Name string
	Arg  string
	Rest string
}

func ParseCommandForTest(text string) (ParsedCommand, bool) {
	p, ok := parseCommand(text)
	return ParsedCommand{Name: p.name, Arg: p.arg, Rest: p.rest}, ok
}

type CommandSpec struct {
	Name        string
	Description string
	Category    string
	HasArg      bool
}

func AllCommandSpecsForTest() []CommandSpec {
	out := make([]CommandSpec, len(registry))
	for i, s := range registry {
		out[i] = CommandSpec{
			Name:        s.Name,
			Description: s.Description,
			Category:    s.Category,
			HasArg:      s.HasArg,
		}
	}
	return out
}

var _ tea.Cmd
var _ client_sdk.Event