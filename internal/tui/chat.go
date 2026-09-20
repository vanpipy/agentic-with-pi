package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
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

type chatMsg struct {
	role      role
	text      string
	toolCalls []toolCallInline
	duration  time.Duration
	usage     *msgUsage
	promptNum int
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
	messages   []chatMsg
	streaming  strings.Builder
	reasoning  strings.Builder
	width      int
	height     int
	following  bool
	promptNum  int
}

func newChatModel() *chatModel {
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	vp.MouseWheelEnabled = true
	vp.SoftWrap = true
	return &chatModel{
		viewport:  vp,
		following: true,
	}
}

func (c *chatModel) SetSize(w, h int) {
	c.width = w
	c.height = h
	c.viewport.SetWidth(w)
	c.viewport.SetHeight(h)
	c.refresh()
}

func (c *chatModel) Update(msg tea.Msg) (tea.Cmd, bool) {
	vp, cmd := c.viewport.Update(msg)
	c.viewport = vp
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
		lines = append(lines, renderMsg(m, c.viewport.Width())...)
	}
	if c.reasoning.Len() > 0 {
		lines = append(lines, renderThinking(c.reasoning.String(), c.viewport.Width())...)
	}
	if c.streaming.Len() > 0 {
		lines = append(lines, renderAssistant(c.streaming.String(), c.viewport.Width())...)
	}
	return strings.Join(lines, "\n")
}

func renderMsg(g chatMsg, width int) []string {
	body := bodyFor(g)
	bodyWidth := width - glyphWidth(g)
	if bodyWidth < 16 {
		bodyWidth = 16
	}
	out := wrapLines(body, bodyWidth)
	out = indentLines(out, glyphFor(g))
	if g.duration > 0 {
		out = append(out, "  ⏱ "+g.duration.Round(time.Millisecond).String())
	}
	if g.usage != nil && g.usage.total > 0 {
		out = append(out, fmt.Sprintf("  ↻ %d → %d  (%d tokens)", g.usage.prompt, g.usage.completion, g.usage.total))
	}
	if g.toolCalls != nil {
		out = append(append(renderToolInline(g.toolCalls), ""), out...)
	}
	return out
}

func renderThinking(text string, width int) []string {
	body := aiThinking.Render(text)
	return wrapAndIndent(body, " ∵ ", width)
}

func renderAssistant(text string, width int) []string {
	rendered := renderMarkdownBody(text, width)
	return wrapAndIndent(rendered, " ✦ ", width)
}

func renderToolInline(calls []toolCallInline) []string {
	if len(calls) == 0 {
		return nil
	}
	parts := make([]string, len(calls))
	for i, c := range calls {
		args := c.args
		if len(args) > 40 {
			args = args[:37] + "..."
		}
		parts[i] = toolName.Render(c.name) + "(" + args + ")"
	}
	label := "tool: "
	if len(calls) > 1 {
		label = "tools: "
	}
	return []string{toolPrefix.Render(label) + strings.Join(parts, toolSeparator.Render(" · "))}
}

func bodyFor(g chatMsg) string {
	switch g.role {
	case roleThinking:
		return aiThinking.Render(g.text)
	case roleAssistant:
		return renderMarkdownBody(g.text, 0)
	}
	return g.text
}

func glyphFor(g chatMsg) string {
	if g.role == roleUser && g.promptNum > 0 {
		return fmt.Sprintf("%d› ", g.promptNum)
	}
	return glyphForRole(g.role)
}

func glyphForRole(r role) string {
	switch r {
	case roleUser:
		return " › "
	case roleAssistant:
		return " ✦ "
	case roleTool:
		return " ⚙ "
	case roleObserve:
		return " ← "
	case roleError:
		return " ✗ "
	case roleSystem:
		return " ⋯ "
	case roleThinking:
		return " ∵ "
	}
	return "   "
}

func glyphWidth(g chatMsg) int {
	return ansiWidth(glyphFor(g))
}

func wrapAndIndent(text, prefix string, width int) []string {
	if width <= 0 {
		return indentLines(strings.Split(text, "\n"), prefix)
	}
	out := wrapLines(text, width-ansiWidth(prefix))
	return indentLines(out, prefix)
}

func wrapLines(text string, width int) []string {
	if width <= 0 {
		return strings.Split(text, "\n")
	}
	var out []string
	for _, line := range strings.Split(text, "\n") {
		for ansiWidth(line) > width {
			cut := cutAtWidth(line, width)
			out = append(out, cut)
			line = line[ansiWidth(cut):]
		}
		out = append(out, line)
	}
	return out
}

func cutAtWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	count := 0
	for i := range s {
		if count == w {
			return s[:i]
		}
		count++
	}
	return s
}

func indentLines(lines []string, prefix string) []string {
	width := ansiWidth(prefix)
	if width == 0 {
		return lines
	}
	indent := strings.Repeat(" ", width)
	out := make([]string, len(lines))
	for i, line := range lines {
		if i == 0 {
			out[i] = prefix + line
		} else {
			out[i] = indent + line
		}
	}
	return out
}

func ansiWidth(s string) int {
	w := 0
	for range s {
		w++
	}
	return w
}

func (c *chatModel) ScrollUp(n int) {
	if n <= 0 {
		return
	}
	c.following = false
	c.viewport.ScrollUp(n)
}

func (c *chatModel) ScrollDown(n int) {
	if n <= 0 {
		return
	}
	c.viewport.ScrollDown(n)
	if c.viewport.AtBottom() {
		c.following = true
	}
}

func (c *chatModel) HalfPageUp() {
	c.following = false
	c.viewport.HalfPageUp()
}

func (c *chatModel) HalfPageDown() {
	c.viewport.HalfPageDown()
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
	target := -1
	for i, m := range c.messages {
		if m.role != roleUser {
			continue
		}
		if direction > 0 && i < c.viewport.YOffset() {
			target = i
		} else if direction < 0 && i >= c.viewport.YOffset() {
			target = i
			break
		}
	}
	if target == -1 {
		return
	}
	c.viewport.SetYOffset(c.lineOffsetForPrompt(target))
	c.following = c.viewport.AtBottom()
}

func (c *chatModel) lineOffsetForPrompt(idx int) int {
	offset := 0
	for i := 0; i < idx; i++ {
		offset += c.lineCount(c.messages[i]) + 1
	}
	total := c.viewport.TotalLineCount()
	maxOffset := total - c.viewport.Height()
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	return offset
}

func (c *chatModel) lineCount(g chatMsg) int {
	return len(renderMsg(g, c.viewport.Width()))
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
	c.promptNum++
	c.messages = append(c.messages, chatMsg{
		role:      roleUser,
		text:      text,
		promptNum: c.promptNum,
	})
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
	c.promptNum = 0
	c.refresh()
}