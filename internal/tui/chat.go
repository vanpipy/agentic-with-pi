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
	linesCache      map[string][]string
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
	c.linesCache = nil
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
	c.linesCache = nil
	c.viewport.SetContent(c.content())
	if c.following {
		c.viewport.GotoBottom()
	}
}

func (c *chatModel) content() string {
	width := c.viewport.Width()
	var lines []string
	for _, m := range c.messages {
		lines = append(lines, c.renderedLines(m, width)...)
	}
	if c.reasoning.Len() > 0 {
		lines = append(lines, renderThinking(c.reasoning.String(), width, false)...)
	}
	if c.streaming.Len() > 0 {
		lines = append(lines, renderAssistant(c.streaming.String(), width)...)
	}
	return strings.Join(lines, "\n")
}

func (c *chatModel) renderedLines(m chatMsg, width int) []string {
	key := linesCacheKey(m, width)
	if c.linesCache == nil {
		c.linesCache = make(map[string][]string)
	}
	if v, ok := c.linesCache[key]; ok {
		return v
	}
	c.lineCountMisses++
	v := renderMsg(m, width)
	c.linesCache[key] = v
	return v
}

func linesCacheKey(m chatMsg, width int) string {
	hasTool := m.toolData != nil
	usageStr := ""
	if m.usage != nil {
		usageStr = fmt.Sprintf("%d,%d,%d", m.usage.prompt, m.usage.completion, m.usage.total)
	}
	return fmt.Sprintf("%d|%d|%v|%v|%d|%d|%s|%s|%s",
		width, m.role, m.collapsed, hasTool, m.promptNum, m.duration, m.text, m.intent, usageStr)
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
	bodyWidth := width - lipgloss.Width(" ✦ ")
	if bodyWidth < 8 {
		bodyWidth = 8
	}
	return renderAssistantSegments(text, layout, bodyWidth)
}

type assistantSegmentKind string

const (
	segmentMarkdown assistantSegmentKind = "markdown"
	segmentPlan     assistantSegmentKind = "plan"
	segmentDiff     assistantSegmentKind = "diff"
	segmentImage    assistantSegmentKind = "image"
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

func expandImageSegments(in []assistantSegment) []assistantSegment {
	if len(in) == 0 {
		return in
	}
	var out []assistantSegment
	for _, seg := range in {
		if seg.kind != segmentMarkdown {
			out = append(out, seg)
			continue
		}
		parts := splitMarkdownForImages(seg.text)
		if len(parts) == 0 {
			out = append(out, seg)
			continue
		}
		out = append(out, parts...)
	}
	return out
}

type imageMatch struct {
	match string
	url   string
}

func splitMarkdownForImages(text string) []assistantSegment {
	lines := strings.Split(text, "\n")
	var out []assistantSegment
	var mdLines []string
	inFence := false
	flushMarkdown := func() {
		if len(mdLines) > 0 {
			out = append(out, assistantSegment{kind: segmentMarkdown, text: strings.Join(mdLines, "\n")})
			mdLines = nil
		}
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			mdLines = append(mdLines, line)
			continue
		}
		if inFence {
			mdLines = append(mdLines, line)
			continue
		}
		rest := line
		for len(rest) > 0 {
			match, prefixLen, ok := scanImageRef(rest)
			if !ok {
				break
			}
			prefix := rest[:prefixLen]
			if prefix != "" {
				mdLines = append(mdLines, prefix)
			}
			flushMarkdown()
			typeName := inferImageType(match.url)
			if match.url != "" && typeName != "" {
				out = append(out, assistantSegment{kind: segmentImage, text: typeName})
			}
			rest = rest[prefixLen+len(match.match):]
		}
		if rest != "" {
			mdLines = append(mdLines, rest)
		} else if len(rest) == 0 && len(line) == 0 {
			mdLines = append(mdLines, "")
		}
	}
	flushMarkdown()
	if len(out) == 0 {
		return nil
	}
	return out
}

func scanImageRef(s string) (imageMatch, int, bool) {
	best := imageMatch{}
	bestPos := -1
	for i := 0; i+1 < len(s); i++ {
		if s[i] != '!' || s[i+1] != '[' {
			continue
		}
		m, ok := parseMarkdownImage(s, i)
		if !ok {
			continue
		}
		best = m
		bestPos = i
		break
	}
	for i := 0; i < len(s); i++ {
		if i > 0 {
			prev := s[i-1]
			if prev != ' ' && prev != '\t' {
				continue
			}
		}
		if !strings.HasPrefix(s[i:], "data:image/") {
			continue
		}
		end := i
		for end < len(s) && !isImageTokenBoundary(s[end]) {
			end++
		}
		if end == i {
			continue
		}
		token := s[i:end]
		if !strings.Contains(token, ";base64,") {
			continue
		}
		typeName := inferImageType(token)
		if typeName == "" {
			continue
		}
		if bestPos == -1 || i < bestPos {
			best = imageMatch{match: token, url: token}
			bestPos = i
		}
		break
	}
	if bestPos == -1 {
		return imageMatch{}, 0, false
	}
	return best, bestPos, true
}

func parseMarkdownImage(s string, start int) (imageMatch, bool) {
	if start+1 >= len(s) || s[start] != '!' || s[start+1] != '[' {
		return imageMatch{}, false
	}
	altStart := start + 2
	altEnd := -1
	j := altStart
	for j < len(s) {
		if s[j] == '\\' && j+1 < len(s) {
			j += 2
			continue
		}
		if s[j] == ']' {
			if j+1 < len(s) && s[j+1] == '(' {
				altEnd = j
				break
			}
		}
		j++
	}
	if altEnd == -1 {
		return imageMatch{}, false
	}
	urlStart := altEnd + 1
	if urlStart >= len(s) || s[urlStart] != '(' {
		return imageMatch{}, false
	}
	urlEnd := -1
	k := urlStart + 1
	for k < len(s) {
		if s[k] == '\\' && k+1 < len(s) {
			k += 2
			continue
		}
		if s[k] == ')' {
			urlEnd = k
			break
		}
		k++
	}
	if urlEnd == -1 {
		return imageMatch{}, false
	}
	url := s[urlStart+1 : urlEnd]
	full := s[start : urlEnd+1]
	return imageMatch{match: full, url: url}, true
}

func isImageTokenBoundary(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ')'
}

func inferImageType(url string) string {
	if url == "" {
		return ""
	}
	if strings.HasPrefix(url, "data:image/") {
		rest := url[len("data:image/"):]
		for i := 0; i < len(rest); i++ {
			c := rest[i]
			if c == ';' || c == ',' {
				return rest[:i]
			}
		}
		return rest
	}
	clean := url
	if idx := strings.IndexAny(clean, "?#"); idx != -1 {
		clean = clean[:idx]
	}
	lastSlash := strings.LastIndex(clean, "/")
	base := clean
	if lastSlash != -1 {
		base = clean[lastSlash+1:]
	}
	lastDot := strings.LastIndex(base, ".")
	if lastDot == -1 || lastDot == len(base)-1 {
		return ""
	}
	ext := base[lastDot+1:]
	if len(ext) == 0 || len(ext) > 5 {
		return ""
	}
	for i := 0; i < len(ext); i++ {
		c := ext[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return ""
		}
	}
	return strings.ToLower(ext)
}

func renderImageBody(typeName string, width int) string {
	if typeName == "" {
		typeName = "image"
	}
	label := "▣ image: " + typeName
	return imagePlaceholder.Width(width).Render(label)
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
	segments = expandImageSegments(segments)
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
		case segmentImage:
			rendered = renderImageBody(seg.text, width)
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

func (c *chatModel) lineCount(g chatMsg) int {
	return len(c.renderedLines(g, c.viewport.Width()))
}

func (c *chatModel) LineCountMissesForTest() int { return c.lineCountMisses }

func (c *chatModel) LinesCacheSizeForTest() int {
	return len(c.linesCache)
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

func (t ChatModelT) ViewForTest() string {
	return t.model.View()
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

func (t ChatModelT) RenderedLinesForTest(g ChatMsg, width int) []string {
	return t.model.renderedLines(toChatMsg(g), width)
}

func (t ChatModelT) ContentForTest() string {
	return t.model.content()
}

func (t ChatModelT) LineCountForTest(g ChatMsg) int {
	return t.model.lineCount(toChatMsg(g))
}

func (t ChatModelT) AppendStreamForTest(text string) {
	t.model.appendStream(text)
}

func (t ChatModelT) LinesCacheSizeForTest() int {
	return t.model.LinesCacheSizeForTest()
}

func (t ChatModelT) LineCountMissesForTest() int {
	return t.model.LineCountMissesForTest()
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
