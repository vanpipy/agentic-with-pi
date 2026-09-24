package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	agentclient "github.com/vanpiyp/awp/internal/agent-client"
	"github.com/vanpiyp/awp/internal/agent-core/skills"
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

type sessionPickerMsg struct {
	items []sessionItem
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
		"\n  available: /quit /new /resume")
	return false, nil
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
