package tui

import (
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
		m.chat.appendSystem(errorPrefix.Render(" unknown command: /" + parsed.name) +
			helpFooter.Render("\n  type /help for available commands"))
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
	m.showHelp = true
}

func (m *Model) cmdTools() {
	lines := []string{
		systemPrefix.Render(" available tools:"),
		"",
		"  " + toolName.Render("read       ") + helpFooter.Render("read files"),
		"  " + toolName.Render("write      ") + helpFooter.Render("write files"),
		"  " + toolName.Render("edit       ") + helpFooter.Render("edit files"),
		"  " + toolName.Render("bash       ") + helpFooter.Render("run shell commands"),
		"  " + toolName.Render("grep       ") + helpFooter.Render("search file contents"),
		"  " + toolName.Render("find       ") + helpFooter.Render("find files by name"),
		"  " + toolName.Render("ls         ") + helpFooter.Render("list directory"),
	}
	m.chat.appendSystem(strings.Join(lines, "\n"))
}

func (m *Model) cmdResume(sessionID string) {
	if sessionID == "" {
		m.chat.appendSystem(helpFooter.Render(" usage: /resume <session_id>"))
		return
	}
	m.resumeSession(sessionID)
}
