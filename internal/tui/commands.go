package tui

import (
	"fmt"
	"strings"
)

type commandDef struct {
	name        string
	description string
	usage       string
	hasArg      bool
}

type parsedCommand struct {
	name string
	arg  string
	rest string
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

func findCommand(name string) (commandDef, bool) {
	if c, ok := builtinCommands()[name]; ok {
		return c, true
	}
	return commandDef{}, false
}

func builtinCommands() map[string]commandDef {
	return map[string]commandDef{
		"quit": {
			name:        "quit",
			description: "exit the TUI",
		},
		"help": {
			name:        "help",
			description: "show available slash commands",
		},
		"new": {
			name:        "new",
			description: "clear chat history, start fresh turn",
		},
		"tools": {
			name:        "tools",
			description: "list available tools",
		},
		"resume": {
			name:        "resume",
			description: "resume a previous session by id",
			usage:       "/resume <session_id>",
			hasArg:      true,
		},
		"clear": {
			name:        "clear",
			description: "clear chat history",
		},
	}
}

func (m *Model) executeCommand(text string) bool {
	parsed, ok := parseCommand(text)
	if !ok {
		return false
	}

	cmd, found := findCommand(parsed.name)
	if !found {
		m.chat.appendSystem(helpStyle.Render(" unknown command: /" + parsed.name) +
			"\n  type /help for available commands")
		return true
	}

	switch cmd.name {
	case "quit":
		m.shutdown()
		return true
	case "help":
		m.cmdHelp()
	case "new":
		m.chat.reset()
		if parsed.arg != "" {
			m.submit(parsed.arg)
		}
	case "clear":
		m.chat.reset()
	case "tools":
		m.cmdTools()
	case "resume":
		m.cmdResume(parsed.arg)
	}
	return true
}

func (m *Model) cmdHelp() {
	var lines []string
	lines = append(lines, helpStyle.Render(" available slash commands:"))
	lines = append(lines, "")
	for _, c := range builtinCommands() {
		usage := "/" + c.name
		if c.usage != "" {
			usage = c.usage
		}
		lines = append(lines, fmt.Sprintf("  %s   %s", helpStyle.Render(usage), c.description))
	}
	lines = append(lines, "")
	m.chat.appendSystem(strings.Join(lines, "\n"))
}

func (m *Model) cmdTools() {
	lines := []string{
		helpStyle.Render(" available tools:"),
		"",
		"  read       read files",
		"  write      write files",
		"  edit       edit files",
		"  bash       run shell commands",
		"  grep       search file contents",
		"  find       find files by name",
		"  ls         list directory",
	}
	m.chat.appendSystem(strings.Join(lines, "\n"))
}

func (m *Model) cmdResume(sessionID string) {
	if sessionID == "" {
		m.chat.appendSystem(helpStyle.Render(" usage: /resume <session_id>"))
		return
	}
	m.resumeSession(sessionID)
}
