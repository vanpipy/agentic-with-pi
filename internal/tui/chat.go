package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

type role int

const (
	roleUser role = iota
	roleAssistant
	roleTool
	roleObserve
	roleError
	roleSystem
	roleThinking
)

func (r role) String() string {
	switch r {
	case roleUser:
		return "you"
	case roleAssistant:
		return "ai"
	case roleTool:
		return "tool"
	case roleObserve:
		return "obs"
	case roleError:
		return "err"
	}
	return "?"
}

type chatMsg struct {
	role      role
	text      string
	toolCalls []toolCallInline
	duration  time.Duration
	collapsed bool
	usage     *msgUsage
}

type toolCallInline struct {
	name string
	args string
}

type msgUsage struct {
	prompt     int
	completion int
	total      int
}

type chatModel struct {
	viewport  viewport.Model
	messages  []chatMsg
	streaming strings.Builder
	reasoning strings.Builder
	width     int
	height    int
	following bool
}

func newChatModel() *chatModel {
	vp := viewport.New(80, 20)
	vp.MouseWheelEnabled = true
	return &chatModel{
		viewport:  vp,
		following: true,
	}
}

func (c *chatModel) SetSize(w, h int) {
	c.width = w
	c.height = h
	c.viewport.Width = w
	c.viewport.Height = h
	c.refresh()
}

func (c *chatModel) Update(msg tea.Msg) (tea.Cmd, bool) {
	var cmd tea.Cmd
	c.viewport, cmd = c.viewport.Update(msg)
	return cmd, false
}

func (c *chatModel) View() string {
	return c.viewport.View()
}

func (c *chatModel) refresh() {
	c.viewport.SetContent(c.buildContent())
	if c.following {
		c.viewport.GotoBottom()
	}
}

func (c *chatModel) buildContent() string {
	var lines []string
	for _, m := range c.messages {
		lines = append(lines, renderChatMsg(m, c.viewport.Width))
	}
	if c.reasoning.Len() > 0 {
		lines = append(lines, renderChatMsg(chatMsg{
			role:      roleThinking,
			text:      c.reasoning.String(),
			collapsed: false,
		}, c.viewport.Width))
	}
	if c.streaming.Len() > 0 {
		lines = append(lines, renderChatMsg(chatMsg{
			role: roleAssistant,
			text: c.streaming.String(),
		}, c.viewport.Width))
	}
	return strings.Join(lines, "\n")
}

func (c *chatModel) ScrollUp(n int) {
	if n <= 0 {
		return
	}
	c.following = false
	c.viewport.LineUp(n)
}

func (c *chatModel) ScrollDown(n int) {
	if n <= 0 {
		return
	}
	c.viewport.LineDown(n)
	if c.viewport.AtBottom() {
		c.following = true
	}
}

func (c *chatModel) HalfPageUp() {
	c.following = false
	c.viewport.HalfViewUp()
}

func (c *chatModel) HalfPageDown() {
	c.viewport.HalfViewDown()
	if c.viewport.AtBottom() {
		c.following = true
	}
}

func (c *chatModel) GotoTop() {
	c.following = false
	c.viewport.GotoTop()
}

func (c *chatModel) GotoBottom() {
	c.viewport.GotoBottom()
	c.following = true
}

func (c *chatModel) JumpToPrompt(direction int) {
	if direction == 0 {
		return
	}
	var promptIdx int = -1
	if direction > 0 {
		currentVisible := c.firstVisibleMsgIndex()
		for i, m := range c.messages {
			if i >= currentVisible {
				break
			}
			if m.role == roleUser {
				promptIdx = i
			}
		}
	} else {
		currentVisible := c.firstVisibleMsgIndex()
		for i := currentVisible; i < len(c.messages); i++ {
			if c.messages[i].role == roleUser {
				promptIdx = i
				break
			}
		}
	}
	if promptIdx == -1 {
		return
	}
	lineOffset := c.lineOffsetForMsg(promptIdx)
	c.viewport.SetYOffset(lineOffset)
	c.following = c.viewport.AtBottom()
}

func (c *chatModel) firstVisibleMsgIndex() int {
	yOffset := c.viewport.YOffset
	totalLines := c.totalRenderedLines()
	linesFromBottom := totalLines - c.viewport.Height - yOffset
	if linesFromBottom < 0 {
		linesFromBottom = 0
	}
	return c.lineIndexToMsgIndex(linesFromBottom)
}

func (c *chatModel) totalRenderedLines() int {
	return c.viewport.TotalLineCount()
}

func (c *chatModel) lineIndexToMsgIndex(lineIdx int) int {
	idx := 0
	cur := 0
	for _, m := range c.messages {
		lines := c.lineCountForMsg(m)
		if lineIdx >= cur && lineIdx < cur+lines {
			return idx
		}
		cur += lines + 1
		idx++
	}
	if len(c.messages) > 0 {
		return len(c.messages) - 1
	}
	return 0
}

func (c *chatModel) lineOffsetForMsg(msgIdx int) int {
	offset := 0
	for i := 0; i < msgIdx && i < len(c.messages); i++ {
		offset += c.lineCountForMsg(c.messages[i]) + 1
	}
	total := c.viewport.TotalLineCount()
	maxOffset := total - c.viewport.Height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	return offset
}

func (c *chatModel) lineCountForMsg(m chatMsg) int {
	w := c.viewport.Width
	if w <= 0 {
		w = 80
	}
	rendered := renderChatMsg(m, w)
	if rendered == "" {
		return 0
	}
	return strings.Count(rendered, "\n") + 1
}

func (c *chatModel) AtBottom() bool {
	return c.viewport.AtBottom()
}

func (c *chatModel) IsFollowing() bool {
	return c.following
}

func (c *chatModel) submit(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	c.messages = append(c.messages, chatMsg{role: roleUser, text: text})
	c.refresh()
}

func (c *chatModel) appendStream(text string) {
	c.streaming.WriteString(text)
	c.refresh()
}

func (c *chatModel) appendReasoning(text string) {
	c.reasoning.WriteString(text)
	c.refresh()
}

func (c *chatModel) appendTool(name, args, result string) {
	if result != "" {
		c.messages = append(c.messages, chatMsg{
			role: roleTool,
			text: fmt.Sprintf("%s(%s) → %s", name, args, result),
		})
	} else {
		c.messages = append(c.messages, chatMsg{
			role: roleTool,
			text: fmt.Sprintf("%s(%s)", name, args),
		})
	}
	c.refresh()
}

func (c *chatModel) appendObserve(text string) {
	c.messages = append(c.messages, chatMsg{role: roleObserve, text: text})
	c.refresh()
}

func (c *chatModel) appendError(text string) {
	c.messages = append(c.messages, chatMsg{role: roleError, text: text})
	c.refresh()
}

func (c *chatModel) appendSystem(text string) {
	c.messages = append(c.messages, chatMsg{role: roleSystem, text: text})
	c.refresh()
}

func (c *chatModel) commitStream() {
	if c.reasoning.Len() > 0 {
		c.messages = append(c.messages, chatMsg{
			role: roleThinking,
			text: c.reasoning.String(),
		})
		c.reasoning.Reset()
	}
	if c.streaming.Len() > 0 {
		c.messages = append(c.messages, chatMsg{
			role: roleAssistant,
			text: c.streaming.String(),
		})
		c.streaming.Reset()
	}
	c.refresh()
}

func (c *chatModel) discardStream() {
	c.streaming.Reset()
	c.reasoning.Reset()
	c.refresh()
}

func (c *chatModel) reset() {
	c.messages = nil
	c.streaming.Reset()
	c.reasoning.Reset()
	c.following = true
	c.refresh()
}

func renderChatMsg(m chatMsg, width int) string {
	prefix := ""
	switch m.role {
	case roleUser:
		prefix = youStyle.Render("> you")
	case roleAssistant:
		prefix = assistantStyle.Render("> ai ")
	case roleTool:
		prefix = toolStyle.Render("> ⚙  ")
	case roleObserve:
		prefix = assistantStyle.Render("> ←  ")
	case roleError:
		prefix = errorStyle.Render("> ✗  ")
	case roleSystem:
		prefix = helpStyle.Render("> ⋯  ")
	case roleThinking:
		if m.collapsed {
			prefix = thinkingStyle.Render("> ∵  ")
		} else {
			prefix = thinkingStyle.Render("> ∵  ")
		}
	}

	bodyWidth := width - 4
	if bodyWidth < 16 {
		bodyWidth = 16
	}

	var body string
	switch m.role {
	case roleThinking:
		body = renderThinkingBody(m, bodyWidth)
	case roleAssistant, roleSystem, roleTool, roleObserve, roleError:
		body = renderMarkdownBody(m.text, bodyWidth)
	case roleUser:
		body = wrapText(m.text, bodyWidth)
	default:
		body = wrapText(m.text, bodyWidth)
	}

	toolInline := renderToolInline(m.toolCalls, bodyWidth)
	if toolInline != "" {
		body = toolInline + "\n" + body
	}

	result := fmt.Sprintf("%s  %s", prefix, body)
	if m.duration > 0 {
		result += "\n" + helpStyle.Render(fmt.Sprintf("  ⏱ %s", m.duration.Round(time.Millisecond)))
	}
	if m.usage != nil && m.usage.total > 0 {
		result += "\n" + helpStyle.Render(fmt.Sprintf("  ↻ %d → %d  (%d tokens)", m.usage.prompt, m.usage.completion, m.usage.total))
	}
	return result
}

func renderThinkingBody(m chatMsg, width int) string {
	if m.collapsed {
		dur := m.duration.Round(time.Second)
		if dur == 0 {
			dur = m.duration.Round(time.Millisecond)
		}
		return helpStyle.Render(fmt.Sprintf("▸ thought for %s", dur))
	}
	return thinkingStyle.Render(wrapText(m.text, width))
}

func renderToolInline(calls []toolCallInline, width int) string {
	if len(calls) == 0 {
		return ""
	}
	parts := make([]string, len(calls))
	for i, c := range calls {
		args := c.args
		if len(args) > 40 {
			args = args[:37] + "..."
		}
		parts[i] = fmt.Sprintf("%s(%s)", c.name, args)
	}
	label := "tool: "
	if len(calls) > 1 {
		label = "tools: "
	}
	prefix := toolStyle.Render(label)
	return prefix + strings.Join(parts, " · ")
}

var (
	mdRenderer   *glamour.TermRenderer
	mdRendererMu sync.Mutex
)

func renderMarkdownBody(text string, width int) string {
	if text == "" {
		return ""
	}
	mdRendererMu.Lock()
	defer mdRendererMu.Unlock()

	if mdRenderer == nil {
		r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle("notty"),
			glamour.WithWordWrap(width),
			glamour.WithEmoji(),
		)
		if err != nil {
			return wrapText(text, width)
		}
		mdRenderer = r
	}

	out, err := mdRenderer.Render(text)
	if err != nil {
		return wrapText(text, width)
	}
	return lipgloss.NewStyle().
		Width(width).
		Render(strings.TrimRight(out, "\n"))
}

func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}
	var out strings.Builder
	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			out.WriteString("\n")
		}
		for len(line) > width {
			out.WriteString(line[:width])
			out.WriteString("\n")
			line = line[width:]
		}
		out.WriteString(line)
	}
	return out.String()
}
