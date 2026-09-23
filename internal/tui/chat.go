package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/vanpiyp/awp/internal/agent-core/tools"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
)

type chatModel struct {
	viewport        viewport.Model
	messages        []chatMsg
	streaming       strings.Builder
	reasoning       strings.Builder
	width           int
	height          int
	following       bool
	promptNum       int
	lineCountCache  map[lineCountKey]int
	lineCountMisses int
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
	c.lineCountCache = nil
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
	c.lineCountCache = nil
	c.viewport.SetContent(c.content())
	if c.following {
		c.viewport.GotoBottom()
	}
}

func (c *chatModel) content() string {
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
	hints := hintLines(g)
	if len(hints) > 0 {
		indented = append(indented, hints...)
	}
	return indented
}

func (g chatMsg) body(layout roleLayout, width int) []string {
	if g.role == roleAssistant {
		return renderAssistantSegments(g.text, layout, width)
	}
	if g.role == roleTool && g.toolData != nil {
		return renderToolCard(g.toolData, g.collapsed, g.duration, width)
	}
	if g.role == roleThinking && g.collapsed {
		dur := g.duration.Round(time.Second)
		if dur == 0 {
			dur = g.duration.Round(time.Millisecond)
		}
		return []string{layout.body.Render(fmt.Sprintf("▸ thought for %s", dur))}
	}
	if g.role == roleObserve && g.collapsed {
		summary := g.intent
		if summary == "" {
			summary = "result"
		}
		return wrapRender(layout.body.Width(width), "▸ "+truncateMid(summary, 60))
	}
	return wrapRender(layout.body.Width(width), g.text)
}

func renderToolCard(td *json_rpc.MessageContentPart, collapsed bool, dur time.Duration, width int) []string {
	if width < 8 {
		width = 8
	}
	var body strings.Builder
	body.WriteString(toolCardHeader(td, dur))
	body.WriteByte('\n')
	if collapsed {
		body.WriteString("▸ result")
	} else {
		body.WriteString(toolCardArgs.Render(tools.TruncateMiddle(string(td.Arguments), 60)))
	}
	rendered := toolCard.Width(width).Render(body.String())
	if rendered == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(rendered, "\n"), "\n")
}

func toolCardHeader(td *json_rpc.MessageContentPart, dur time.Duration) string {
	var b strings.Builder
	b.WriteString("⚙ ")
	if td.Intent != "" {
		b.WriteString(td.Intent)
		b.WriteString("  ")
	}
	b.WriteString(td.Name)
	if dur > 0 {
		b.WriteString("  ")
		b.WriteString(durationHint.Render("⏱ " + dur.Round(time.Millisecond).String()))
	}
	return b.String()
}

func truncateMid(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
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

type assistantSegmentKind string

const (
	segmentMarkdown assistantSegmentKind = "markdown"
	segmentPlan     assistantSegmentKind = "plan"
	segmentDiff     assistantSegmentKind = "diff"
)

type assistantSegment struct {
	kind assistantSegmentKind
	text string
}

func splitAssistantSegments(text string) []assistantSegment {
	var segments []assistantSegment
	var pending []string
	state := segmentMarkdown
	flush := func(kind assistantSegmentKind) {
		if len(pending) > 0 {
			segments = append(segments, assistantSegment{kind: kind, text: strings.Join(pending, "\n")})
			pending = nil
		}
	}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if state == segmentMarkdown {
			switch trimmed {
			case "```plan":
				flush(segmentMarkdown)
				state = segmentPlan
				continue
			case "```diff":
				flush(segmentMarkdown)
				state = segmentDiff
				continue
			}
			pending = append(pending, line)
			continue
		}
		if trimmed == "```" {
			wasEmpty := len(pending) == 0
			flush(state)
			if wasEmpty {
				segments = append(segments, assistantSegment{kind: state, text: ""})
			}
			state = segmentMarkdown
			continue
		}
		pending = append(pending, line)
	}
	if state != segmentMarkdown {
		opening := "```" + string(state)
		full := append([]string{opening}, pending...)
		segments = append(segments, assistantSegment{kind: segmentMarkdown, text: strings.Join(full, "\n")})
	} else {
		flush(segmentMarkdown)
	}
	return segments
}

func splitAssistantPlanSegments(text string) []assistantSegment {
	return splitAssistantSegments(text)
}

func renderDiffBody(text string) string {
	if text == "" {
		return ""
	}
	var b strings.Builder
	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		switch {
		case strings.HasPrefix(line, "+"):
			b.WriteString(diffAdd.Render(line))
		case strings.HasPrefix(line, "-"):
			b.WriteString(diffRemove.Render(line))
		case strings.HasPrefix(line, " "):
			b.WriteString(diffContext.Render(line))
		default:
			b.WriteString(diffNeutral.Render(line))
		}
	}
	return b.String()
}

func renderAssistantSegments(text string, layout roleLayout, width int) []string {
	if width < 8 {
		width = 8
	}
	segments := splitAssistantSegments(text)
	if len(segments) == 0 {
		return nil
	}
	var out []string
	for _, seg := range segments {
		var rendered string
		switch seg.kind {
		case segmentPlan:
			inner := renderMarkdownBody(seg.text, width)
			rendered = planBlock.Width(width).Render(inner)
		case segmentDiff:
			rendered = diffBlock.Width(width).Render(renderDiffBody(seg.text))
		default:
			md := renderMarkdownBody(seg.text, width)
			rendered = layout.body.Width(width).Render(md)
		}
		if rendered == "" {
			continue
		}
		out = append(out, strings.Split(strings.TrimRight(rendered, "\n"), "\n")...)
	}
	return out
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

func (c *chatModel) ToggleCollapseAtViewportTop() {
	top := c.viewport.YOffset()
	walked := 0
	for i, m := range c.messages {
		h := c.lineCount(m)
		if top >= walked && top < walked+h {
			if m.role == roleTool || m.role == roleThinking {
				c.messages[i].collapsed = !c.messages[i].collapsed
				c.refresh()
			}
			return
		}
		walked += h
	}
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

func (c *chatModel) AtBottomExisting() bool {
	return c.viewport.AtBottom()
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
	target := -1
	lastUserIdx := -1
	for i := len(c.messages) - 1; i >= 0; i-- {
		if c.messages[i].role == roleUser && c.messages[i].promptNum > 0 {
			lastUserIdx = i
			break
		}
	}
	if lastUserIdx == -1 {
		return
	}

	currentPrompt := c.promptNumAtViewportTop()
	switch {
	case direction > 0:
		if c.following {
			target = lastUserIdx
			c.following = false
		} else {
			for i := lastUserIdx; i >= 0; i-- {
				m := c.messages[i]
				if m.role != roleUser || m.promptNum == 0 {
					continue
				}
				if m.promptNum < currentPrompt {
					target = i
					break
				}
			}
		}
	case direction < 0:
		if !c.following {
			return
		}
		target = lastUserIdx
		if target == -1 {
			c.GotoBottom()
			return
		}
	}
	if target == -1 {
		return
	}
	wasFollowing := c.following
	c.scrollToMessage(target)
	if wasFollowing {
		c.following = false
	} else {
		c.following = c.viewport.AtBottom()
	}
}

func (c *chatModel) scrollToMessage(idx int) {
	totalLines := 0
	for _, m := range c.messages {
		totalLines += c.lineCount(m)
	}
	height := c.viewport.Height()
	if totalLines <= height {
		return
	}
	maxOffset := totalLines - height
	offset := 0
	for i := 0; i < idx; i++ {
		offset += c.lineCount(c.messages[i])
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	c.viewport.SetYOffset(offset)
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

func (c *chatModel) scrollYOffsetFor(idx int) int {
	offset := 0
	for i := 0; i < idx; i++ {
		offset += c.lineCount(c.messages[i])
	}
	return offset
}

type lineCountKey struct {
	width     int
	role      role
	text      string
	intent    string
	duration  time.Duration
	usage     *msgUsage
	promptNum int
	collapsed bool
}

func lineCountKeyFrom(g chatMsg, width int) lineCountKey {
	return lineCountKey{
		width:     width,
		role:      g.role,
		text:      g.text,
		intent:    g.intent,
		duration:  g.duration,
		usage:     g.usage,
		promptNum: g.promptNum,
		collapsed: g.collapsed,
	}
}

func (c *chatModel) lineCount(g chatMsg) int {
	width := c.viewport.Width()
	key := lineCountKeyFrom(g, width)
	if c.lineCountCache != nil {
		if v, ok := c.lineCountCache[key]; ok {
			return v
		}
	} else {
		c.lineCountCache = make(map[lineCountKey]int)
	}
	c.lineCountMisses++
	v := len(renderMsg(g, width))
	c.lineCountCache[key] = v
	return v
}

func (c *chatModel) LineCountMissesForTest() int { return c.lineCountMisses }

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

func (c *chatModel) appendTool(part json_rpc.MessageContentPart) {
	prefix := intentPrefix(part.Intent)
	c.messages = append(c.messages, chatMsg{
		role:     roleTool,
		text:     fmt.Sprintf("%s%s(%s)", prefix, part.Name, string(part.Arguments)),
		toolData: &part,
	})
	c.refresh()
}

func shouldCollapseResult(result string) bool {
	if strings.Count(result, "\n") >= 5 {
		return true
	}
	if len(result) > 500 {
		return true
	}
	return false
}

func intentPrefix(intent string) string {
	if intent == "" {
		return ""
	}
	return intent + " · "
}

func (c *chatModel) appendObserve(text, intent string) {
	preview := text
	if tools.ToolOutputLooksFailed(text) {
		if summary, ok := tools.ConciseToolErrorSummary(text); ok {
			preview = summary
		}
	} else {
		preview = tools.TruncateMiddle(text, 120)
	}
	c.messages = append(c.messages, chatMsg{
		role:      roleObserve,
		text:      preview,
		intent:    intent,
		collapsed: shouldCollapseResult(text),
	})
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
	Intent    string
	Duration  time.Duration
	Usage     *msgUsage
	PromptNum int
	Collapsed bool
	Title     string
	ToolCalls []string
	ToolData  *json_rpc.MessageContentPart
}

func toChatMsg(c ChatMsg) chatMsg {
	return chatMsg{
		role:      c.Role,
		text:      c.Text,
		intent:    c.Intent,
		duration:  c.Duration,
		usage:     c.Usage,
		promptNum: c.PromptNum,
		collapsed: c.Collapsed,
		title:     c.Title,
		toolCalls: c.ToolCalls,
		toolData:  c.ToolData,
	}
}

func fromChatMsg(m chatMsg) ChatMsg {
	return ChatMsg{
		Role:      m.role,
		Text:      m.text,
		Intent:    m.intent,
		Duration:  m.duration,
		Usage:     m.usage,
		PromptNum: m.promptNum,
		Collapsed: m.collapsed,
		Title:     m.title,
		ToolCalls: m.toolCalls,
		ToolData:  m.toolData,
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

func ChatMsgFromTest(c ChatMsg) ChatMsg {
	return fromChatMsg(toChatMsg(c))
}

func AppendToolForTest(t ChatModelT, part json_rpc.MessageContentPart) {
	t.model.appendTool(part)
}

func LastChatMsgForTest(t ChatModelT) ChatMsg {
	m := t.model
	if len(m.messages) == 0 {
		return ChatMsg{}
	}
	return fromChatMsg(m.messages[len(m.messages)-1])
}
