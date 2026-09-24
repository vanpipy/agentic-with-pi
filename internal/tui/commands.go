package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	agentclient "github.com/vanpiyp/awp/internal/agent-client"
	"github.com/vanpiyp/awp/internal/agent-core/skills"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
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
		Name:        "new",
		Description: "clear chat history, start fresh turn",
		Category:    "session",
		Run: func(m *Model, _ string) (bool, tea.Cmd) {
			m.chat.reset()
			return false, nil
		},
	},
	{
		Name:        "resume",
		Description: "resume a previous session",
		Category:    "session",
		Usage:       "/resume",
		Run: func(m *Model, arg string) (bool, tea.Cmd) {
			if strings.TrimSpace(arg) != "" {
				return false, m.startResume(arg)
			}
			return false, m.showSessionPicker()
		},
	},
	{
		Name:        "compact",
		Description: "force-trigger context compaction",
		Category:    "session",
		HasArg:      false,
		Usage:       "/compact [--force]",
		Run: func(m *Model, arg string) (bool, tea.Cmd) {
			force := strings.Contains(arg, "--force")
			return false, m.runCompact(force)
		},
	},
	{
		Name:        "usage",
		Description: "show token usage for current session",
		Category:    "session",
		HasArg:      false,
		Usage:       "/usage",
		Run: func(m *Model, _ string) (bool, tea.Cmd) {
			return false, m.showUsage()
		},
	},
	{
		Name:        "skills",
		Description: "list loaded skills",
		Category:    "info",
		HasArg:      false,
		Usage:       "/skills",
		Run: func(m *Model, _ string) (bool, tea.Cmd) {
			m.chat.appendSystem(renderSkillsList(skillRegistry))
			return false, nil
		},
	},
}

func (m *Model) startResume(sessionID string) tea.Cmd {
	if m.conn == nil {
		return nil
	}
	events, err := m.conn.Resume(context.Background(), sessionID)
	if err != nil {
		m.chat.appendError("resume: " + err.Error())
		return nil
	}
	m.session = sessionID
	m.chat.reset()
	m.state = StateStreaming
	m.events = events
	return m.readNextEvent()
}

func (m *Model) showSessionPicker() tea.Cmd {
	if m.conn == nil {
		return nil
	}
	return func() tea.Msg {
		summaries, err := m.conn.ListSessions(context.Background())
		if err != nil {
			return errMsg{err}
		}
		items := make([]sessionItem, len(summaries))
		for i, s := range summaries {
			items[i] = sessionItem{
				id:        s.SessionID,
				model:     s.Model,
				startedAt: s.StartedAt,
				events:    s.Events,
			}
		}
		return sessionPickerMsg{items: items}
	}
}

func (m *Model) runCompact(force bool) tea.Cmd {
	if m.conn == nil {
		m.chat.appendError("compact: no active connection")
		return nil
	}
	sessionID := m.session
	conn := m.conn
	return func() tea.Msg {
		res, err := conn.Compact(context.Background(), sessionID, force)
		if err != nil {
			return errMsg{fmt.Errorf("compact: %w", err)}
		}
		return compactDoneMsg{result: res, sessionID: sessionID}
	}
}

func (m *Model) showUsage() tea.Cmd {
	sessionID := m.session
	hint := " Usage stats: no active session — start a prompt to begin tracking."
	if sessionID != "" {
		hint = fmt.Sprintf(" Usage stats: see ~/.awp/logs/sessions/%s.jsonl for this session (%s).", sessionID, sessionID)
	}
	m.chat.appendSystem(hint)
	return nil
}

type sessionPickerMsg struct {
	items []sessionItem
}

type compactDoneMsg struct {
	result    json_rpc.CompactResult
	sessionID string
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
	if found {
		return spec.Run(m, parsed.arg)
	}

	if skill, prompt, ok := matchSkillFor(text); ok {
		return m.executeSkill(skill, prompt)
	}

	m.chat.appendSystem(errorPrefix.Render(" unknown command: /"+parsed.name) +
		"\n  available: " + availableCommandsHint())
	return false, nil
}

func availableCommandsHint() string {
	parts := make([]string, 0, len(registry))
	for _, c := range registry {
		parts = append(parts, "/"+c.Name)
	}
	return strings.Join(parts, " ")
}

func renderSkillsList(reg *skills.Registry) string {
	if reg == nil || len(reg.Skills) == 0 {
		return " loaded skills: (no skills)"
	}
	names := reg.Names()
	parts := make([]string, 0, len(names))
	for _, n := range names {
		s := reg.Skills[n]
		if s != nil && s.Description != "" {
			parts = append(parts, "/"+n+" — "+s.Description)
		} else {
			parts = append(parts, "/"+n)
		}
	}
	return " loaded skills:\n  " + strings.Join(parts, "\n  ")
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

var skillRegistry *skills.Registry

func SetSkillRegistry(r *skills.Registry) {
	skillRegistry = r
}

func SkillRegistryForTest() *skills.Registry {
	return skillRegistry
}

func LoadSkillsForCwdForTest(cwd, homeDir string) {
	loadSkillsForCwd(cwd, homeDir)
}

func loadSkillsForCwd(cwd, homeDir string) {
	reg, err := skills.LoadForCwd(cwd, homeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "skills: load failed: %v\n", err)
		SetSkillRegistry(nil)
		return
	}
	SetSkillRegistry(reg)
	fmt.Fprintf(os.Stderr, "skills: loaded %d (", len(reg.Skills))
	for i, name := range reg.Names() {
		if i > 0 {
			fmt.Fprint(os.Stderr, ", ")
		}
		fmt.Fprintf(os.Stderr, "/%s", name)
	}
	fmt.Fprint(os.Stderr, ")")
	for _, e := range reg.Errors {
		fmt.Fprintf(os.Stderr, "; %v", e)
	}
	fmt.Fprintln(os.Stderr)
}

func matchSkillFor(text string) (*skills.Skill, string, bool) {
	if skillRegistry == nil {
		return nil, "", false
	}
	name, prompt, ok := skillRegistry.ResolveInvocation(text)
	if !ok {
		return nil, "", false
	}
	s, ok := skillRegistry.Get(name)
	if !ok {
		return nil, "", false
	}
	return s, prompt, true
}

func renderSkillPrompt(skill *skills.Skill, userPrompt string) string {
	body := strings.TrimSpace(skill.Content)
	user := strings.TrimSpace(userPrompt)

	var b strings.Builder
	if body != "" {
		b.WriteString("[skill: ")
		b.WriteString(skill.Name)
		b.WriteString("]\n\n")
		b.WriteString(body)
	}
	if user != "" {
		if body != "" {
			b.WriteString("\n\n")
		}
		b.WriteString(user)
	}
	return b.String()
}

func (m *Model) executeSkill(skill *skills.Skill, userPrompt string) (bool, tea.Cmd) {
	rendered := renderSkillPrompt(skill, userPrompt)
	if rendered == "" {
		return false, nil
	}
	m.chat.submit(rendered)
	m.chat.GotoBottom()
	m.lastPrompt = rendered
	m.state = StateStreaming
	return false, m.startStream(rendered)
}

func ExecuteCommandForTest(m *Model, text string) (bool, tea.Cmd) {
	return m.executeCommand(text)
}

func ChatMessagesForTest(m *Model) []string {
	if m == nil || m.chat == nil {
		return nil
	}
	out := make([]string, 0, len(m.chat.messages))
	for _, msg := range m.chat.messages {
		out = append(out, msg.text)
	}
	return out
}

func (m *Model) ChatMessagesForTest() []string {
	return ChatMessagesForTest(m)
}

func (m *Model) LastChatMessageForTest() string {
	msgs := ChatMessagesForTest(m)
	if len(msgs) == 0 {
		return ""
	}
	return msgs[len(msgs)-1]
}

func (m *Model) LastPromptForTest() string {
	if m == nil {
		return ""
	}
	return m.lastPrompt
}

func SetLastPromptForTest(m *Model, val string) {
	if m == nil {
		return
	}
	m.lastPrompt = val
}

var _ tea.Cmd
var _ agentclient.Event
