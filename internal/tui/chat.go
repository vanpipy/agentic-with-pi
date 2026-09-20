package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	tea "charm.land/bubbletea/v2"
)

type chatModel struct {
	viewport viewport.Model
	messages  []chatMsg
	streaming strings.Builder
	reasoning strings.Builder
	width     int
	height    int
	following bool
	promptNum int
}

func newChatModel() *chatModel {
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	vp.MouseWheelEnabled = true
	vp.SoftWrap = true
	c := &chatModel{
		viewport:  vp,
		following: true,
	}
	return c
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
		lines = append(lines, renderThinking(c.reasoning.String(), c.viewport.Width(), false)...)
	}
	if c.streaming.Len() > 0 {
		lines = append(lines, renderAssistant(c.streaming.String(), c.viewport.Width())...)
	}
	return strings.Join(lines, "\n")
}

type roleLayout struct {
	body  lipgloss.Style
	glyph string
}

func roleLayoutFor(r role) roleLayout {
	switch r {
	case roleUser:
		return roleLayout{body: userPromptText, glyph: " › "}
	case roleAssistant:
		return roleLayout{body: aiText, glyph: " ✦ "}
	case roleTool:
		return roleLayout{body: lipgloss.NewStyle(), glyph: " ⚙ "}
	case roleObserve:
		return roleLayout{body: lipgloss.NewStyle(), glyph: " ← "}
	case roleError:
		return roleLayout{body: errorPrefix, glyph: " ✗ "}
	case roleSystem:
		return roleLayout{body: systemPrefix, glyph: " ⋯ "}
	case roleThinking:
		return roleLayout{body: aiThinking, glyph: " ∵ "}
	}
	return roleLayout{}
}

func glyphPrefix(layout roleLayout, role role, promptNum int) string {
	if role == roleUser && promptNum > 0 {
		return userPromptNum.Render(fmt.Sprintf("%d", promptNum)) + layout.glyph
	}
	return layout.glyph
}

func renderMsg(g chatMsg, width int) []string {
	layout := roleLayoutFor(g.role)
	glyph := glyphPrefix(layout, g.role, g.promptNum)
	if width <= 0 {
		width = 80
	}
	glyphWidth := lipgloss.Width(glyph)
	bodyWidth := width - glyphWidth
	if bodyWidth < 8 {
		bodyWidth = 8
	}
	body := g.body(layout, bodyWidth)
	if len(body) == 0 {
		return nil
	}
	indented := prependPrefixAndIndent(body, glyph)
	if g.toolCalls != nil {
		toolLine := renderToolInline(g.toolCalls)
		indented = append(append(toolLine, ""), indented...)
	}
	hints := hintLines(g)
	if len(hints) > 0 {
		indented = append(indented, hints...)
	}
	return indented
}

func (g chatMsg) body(layout roleLayout, width int) []string {
	if g.role == roleAssistant {
		return wrapRender(layout.body.Width(width), renderMarkdownBody(g.text, width))
	}
	if g.role == roleThinking && g.collapsed {
		dur := g.duration.Round(time.Second)
		if dur == 0 {
			dur = g.duration.Round(time.Millisecond)
		}
		return []string{layout.body.Render(fmt.Sprintf("▸ thought for %s", dur))}
	}
	return wrapRender(layout.body.Width(width), g.text)
}

func wrapRender(style lipgloss.Style, text string) []string {
	rendered := style.Render(text)
	if rendered == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(rendered, "\n"), "\n")
}

func renderThinking(text string, width int, collapsed bool) []string {
	layout := roleLayoutFor(roleThinking)
	if collapsed {
		dur := time.Duration(0)
		return wrapRender(layout.body.Width(width),
			fmt.Sprintf("▸ thought for %s", dur.Round(time.Second)))
	}
	return wrapRender(layout.body.Width(width), text)
}

func renderAssistant(text string, width int) []string {
	layout := roleLayoutFor(roleAssistant)
	rendered := renderMarkdownBody(text, width)
	bodyWidth := width - lipgloss.Width(" ✦ ")
	if bodyWidth < 8 {
		bodyWidth = 8
	}
	return wrapRender(layout.body.Width(bodyWidth), rendered)
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

func hintLines(g chatMsg) []string {
	if g.duration > 0 {
		return []string{durationHint.Render("  ⏱ " + g.duration.Round(time.Millisecond).String())}
	}
	if g.usage != nil && g.usage.total > 0 {
		return []string{tokenHint.Render(fmt.Sprintf("  ↻ %d → %d  (%d tokens)", g.usage.prompt, g.usage.completion, g.usage.total))}
	}
	return nil
}

func prependPrefixAndIndent(lines []string, prefix string) []string {
	if prefix == "" || len(lines) == 0 {
		return lines
	}
	prefixWidth := lipgloss.Width(prefix)
	indent := strings.Repeat(" ", prefixWidth)
	out := make([]string, len(lines))
	out[0] = prefix + lines[0]
	for i := 1; i < len(lines); i++ {
		out[i] = indent + lines[i]
	}
	return out
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
	if direction == 0 || len(c.messages) == 0 {
		return
	}
	currentPrompt := c.promptNumAtViewportTop()
	target := -1
	switch {
	case direction > 0:
		for i := len(c.messages) - 1; i >= 0; i-- {
			m := c.messages[i]
			if m.role != roleUser || m.promptNum == 0 {
				continue
			}
			if m.promptNum < currentPrompt {
				target = i
				break
			}
		}
	case direction < 0:
		for i, m := range c.messages {
			if m.role != roleUser || m.promptNum == 0 {
				continue
			}
			if m.promptNum > currentPrompt {
				target = i
				break
			}
		}
	}
	if target == -1 {
		return
	}
	c.viewport.SetYOffset(c.lineOffsetFor(target))
	c.following = c.viewport.AtBottom()
}

func (c *chatModel) promptNumAtViewportTop() int {
	y := c.viewport.YOffset()
	walked := 0
	for _, m := range c.messages {
		h := c.lineCount(m)
		if y < walked+h {
			return m.promptNum
		}
		walked += h
	}
	return c.promptNum
}

func (c *chatModel) lineOffsetFor(idx int) int {
	offset := 0
	for i := 0; i < idx; i++ {
		offset += c.lineCount(c.messages[i])
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

type Role = role

type ChatMsg struct {
	Role      Role
	Text      string
	ToolCalls []toolCallInline
	Duration  time.Duration
	Usage     *msgUsage
	PromptNum int
	Collapsed bool
}

func toChatMsg(c ChatMsg) chatMsg {
	return chatMsg{
		role:      c.Role,
		text:      c.Text,
		toolCalls: c.ToolCalls,
		duration:  c.Duration,
		usage:     c.Usage,
		promptNum: c.PromptNum,
		collapsed: c.Collapsed,
	}
}

type RoleLayout struct {
	body  lipgloss.Style
	glyph string
}

func roleLayoutFromInternal(l roleLayout) RoleLayout {
	return RoleLayout{body: l.body, glyph: l.glyph}
}

func (l RoleLayout) Glyph() string {
	return l.glyph
}

func RoleLayoutForTest(r Role) RoleLayout {
	return roleLayoutFromInternal(roleLayoutFor(r))
}

func RenderMsgForTest(g ChatMsg, width int) []string {
	return renderMsg(toChatMsg(g), width)
}

func GlyphPrefixForTest(layout RoleLayout, role Role, promptNum int) string {
	return glyphPrefix(roleLayout{body: layout.body, glyph: layout.glyph}, role, promptNum)
}

func IndentWrappedLinesForTest(lines []string, prefixWidth int) []string {
	if prefixWidth <= 0 {
		return lines
	}
	indent := strings.Repeat(" ", prefixWidth)
	out := make([]string, len(lines))
	out[0] = indent + lines[0]
	for i := 1; i < len(lines); i++ {
		out[i] = indent + lines[i]
	}
	return out
}

func NewChatModelForTest() ChatModelT {
	return ChatModelT{model: newChatModel()}
}

type ChatModelT struct {
	model *chatModel
}

func (t ChatModelT) SubmitForTest(text string) {
	t.model.submit(text)
}

func (t ChatModelT) PromptNumAtViewportTopForTest() int {
	return t.model.promptNumAtViewportTop()
}

func (t ChatModelT) JumpToPromptForTest(direction int) {
	t.model.JumpToPrompt(direction)
}

func (t ChatModelT) SetSizeForTest(w, h int) {
	t.model.SetSize(w, h)
}