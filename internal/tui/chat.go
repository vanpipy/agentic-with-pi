package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	tea "charm.land/bubbletea/v2"
)

type chatModel struct {
	viewport                  viewport.Model
	messages                   []chatMsg
	streaming                  strings.Builder
	reasoning                  strings.Builder
	width                      int
	height                     int
	following                  bool
	promptNum                  int
	selActive                  bool
	selAnchorMsg               int
	selAnchorCol               int
	selEndMsg                  int
	selEndCol                  int
	dragEdgeFromLastSelection  dragEdge
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

func (c *chatModel) BeginSelection(msgIdx, col int) {
	c.selActive = true
	c.selAnchorMsg = msgIdx
	c.selAnchorCol = col
	c.selEndMsg = msgIdx
	c.selEndCol = col
	c.refresh()
}

func (c *chatModel) ExtendSelection(msgIdx, col int) {
	if !c.selActive {
		c.BeginSelection(msgIdx, col)
		return
	}
	c.selEndMsg = msgIdx
	c.selEndCol = col
	c.refresh()
}

func (c *chatModel) EndSelection() string {
	if !c.selActive {
		return ""
	}
	text := c.ExtractSelectionText()
	c.selActive = false
	c.selAnchorMsg = 0
	c.selAnchorCol = 0
	c.selEndMsg = 0
	c.selEndCol = 0
	c.refresh()
	return text
}

func (c *chatModel) CancelSelection() {
	if !c.selActive {
		return
	}
	c.selActive = false
	c.selAnchorMsg = 0
	c.selAnchorCol = 0
	c.selEndMsg = 0
	c.selEndCol = 0
	c.refresh()
}

func (c *chatModel) IsSelecting() bool { return c.selActive }

func (c *chatModel) HitTest(x, y int) (msgIdx, col int) {
	return c.hitTestInternal(x, y, false)
}

func (c *chatModel) HitTestForDrag(x, y int) (msgIdx, col int) {
	return c.hitTestInternal(x, y, true)
}

func (c *chatModel) hitTestInternal(x, y int, clampToEdges bool) (msgIdx, col int) {
	if x < 0 || y < 0 {
		return -1, 0
	}
	layoutCache := func(role role, promptNum int) int {
		l := roleLayoutFor(role)
		return lipgloss.Width(glyphPrefix(l, role, promptNum))
	}
	line := y
	for idx, m := range c.messages {
		msgLines := renderMsg(m, c.viewport.Width())
		if line < len(msgLines) {
			bodyCol := x - layoutCache(m.role, m.promptNum)
			if bodyCol < 0 {
				bodyCol = 0
			}
			width := lipgloss.Width(msgLines[line])
			if bodyCol > width {
				bodyCol = width
			}
			return idx, bodyCol
		}
		line -= len(msgLines)
	}
	if clampToEdges && len(c.messages) > 0 {
		last := len(c.messages) - 1
		m := c.messages[last]
		msgLines := renderMsg(m, c.viewport.Width())
		if len(msgLines) > 0 {
			bodyCol := x - layoutCache(m.role, m.promptNum)
			if bodyCol < 0 {
				bodyCol = 0
			}
			width := lipgloss.Width(msgLines[len(msgLines)-1])
			if bodyCol > width {
				bodyCol = width
			}
			return last, bodyCol
		}
	}
	return -1, 0
}

type dragEdge int

const (
	edgeNone dragEdge = iota
	edgeTop
	edgeBottom
)

func (c *chatModel) DragEdgeForCoord(y int) dragEdge {
	if !c.selActive {
		return edgeNone
	}
	h := c.viewport.Height()
	if h <= 0 {
		return edgeNone
	}
	zone := edgeZoneRows(h)
	if y < zone && !c.AtTop() {
		return edgeTop
	}
	if y >= h-zone && !c.AtBottom() {
		return edgeBottom
	}
	return edgeNone
}

func edgeZoneRows(height int) int {
	switch {
	case height <= 4:
		return 1
	case height <= 11:
		return 2
	default:
		return 3
	}
}

func (c *chatModel) ScrollByEdge(edge dragEdge) {
	switch edge {
	case edgeTop:
		c.ScrollUp(1)
	case edgeBottom:
		c.ScrollDown(1)
	}
}

func (c *chatModel) AdvanceEdgeScroll() {
	if !c.selActive {
		return
	}
	startMsg, _, _, _, _ := c.selectionRange()
	edge := c.dragEdgeFromLastSelection
	switch edge {
	case edgeTop:
		c.ScrollUp(1)
		if startMsg > 0 {
			c.ExtendSelection(startMsg-1, 0)
		}
	case edgeBottom:
		c.ScrollDown(1)
		c.ExtendSelection(startMsg, c.viewport.Width())
	}
}

func (c *chatModel) AtTop() bool {
	return c.viewport.AtTop()
}

func (c *chatModel) AtBottom() bool {
	return c.viewport.AtBottom()
}
func (c *chatModel) dragEdge() dragEdge {
	return c.dragEdgeFromLastSelection
}

func (c *chatModel) SelectAll() {
	if len(c.messages) == 0 {
		return
	}
	c.selActive = true
	c.selAnchorMsg = 0
	c.selAnchorCol = 0
	c.selEndMsg = len(c.messages) - 1
	last := c.messages[len(c.messages)-1]
	msgLines := renderMsg(last, c.viewport.Width())
	lastLineWidth := 0
	if len(msgLines) > 0 {
		lastLineWidth = lipgloss.Width(msgLines[len(msgLines)-1])
	}
	layout := roleLayoutFor(last.role)
	c.selEndCol = lastLineWidth - lipgloss.Width(glyphPrefix(layout, last.role, last.promptNum))
	if c.selEndCol < 0 {
		c.selEndCol = 0
	}
	c.refresh()
}

func (c *chatModel) ExtractSelectionText() string {
	if !c.selActive {
		return ""
	}
	startMsg, startCol, endMsg, endCol, _ := c.selectionRange()
	if startMsg == endMsg && startCol == endCol {
		return ""
	}
	var parts []string
	for idx := startMsg; idx <= endMsg && idx < len(c.messages); idx++ {
		msg := c.messages[idx]
		msgLines := renderMsg(msg, c.viewport.Width())
		layout := roleLayoutFor(msg.role)
		glyph := glyphPrefix(layout, msg.role, msg.promptNum)
		glyphWidth := lipgloss.Width(glyph)
		for lineIdx, line := range msgLines {
			ls := glyphWidth
			le := textWidth(line)
			if idx == startMsg {
				ls = glyphWidth + startCol
			}
			if idx == endMsg {
				le = glyphWidth + endCol
			}
			if ls >= le {
				continue
			}
			prefix := truncateToCol(line, ls)
			fullEnd := truncateToCol(line, le)
			mid := fullEnd[len(prefix):]
			if lineIdx == 0 {
				mid = ansi.Strip(mid)
			}
			parts = append(parts, strings.TrimRight(mid, " "))
		}
	}
	return strings.Join(parts, "\n")
}

func (c *chatModel) selectionRange() (startMsg, startCol, endMsg, endCol int, ok bool) {
	if !c.selActive {
		return 0, 0, 0, 0, false
	}
	if c.selAnchorMsg < c.selEndMsg ||
		(c.selAnchorMsg == c.selEndMsg && c.selAnchorCol <= c.selEndCol) {
		return c.selAnchorMsg, c.selAnchorCol, c.selEndMsg, c.selEndCol, true
	}
	return c.selEndMsg, c.selEndCol, c.selAnchorMsg, c.selAnchorCol, true
}

func (c *chatModel) buildContent() string {
	var lines []string
	for idx, m := range c.messages {
		msgLines := renderMsg(m, c.viewport.Width())
		if c.selActive {
			startMsg, startCol, endMsg, endCol, _ := c.selectionRange()
			if idx >= startMsg && idx <= endMsg {
				msgLines = applyHighlight(msgLines, idx, m, startMsg, startCol, endMsg, endCol)
			}
		}
		lines = append(lines, msgLines...)
	}
	if c.reasoning.Len() > 0 {
		lines = append(lines, renderThinking(c.reasoning.String(), c.viewport.Width(), false)...)
	}
	if c.streaming.Len() > 0 {
		lines = append(lines, renderAssistant(c.streaming.String(), c.viewport.Width())...)
	}
	return strings.Join(lines, "\n")
}

func applyHighlight(msgLines []string, idx int, m chatMsg, startMsg, startCol, endMsg, endCol int) []string {
	out := make([]string, len(msgLines))
	layout := roleLayoutFor(m.role)
	glyph := glyphPrefix(layout, m.role, m.promptNum)
	glyphWidth := lipgloss.Width(glyph)
	for i, line := range msgLines {
		lineStartCol := glyphWidth
		lineEndCol := textWidth(line)
		if idx == startMsg {
			lineStartCol += startCol
		}
		if idx == endMsg {
			lineEndCol = glyphWidth + endCol
		}
		if lineStartCol >= lineEndCol {
			out[i] = line
			continue
		}
		left := truncateToCol(line, lineStartCol)
		mid := truncateToCol(line, lineEndCol)
		if idx == startMsg && i == 0 && len(left) > 0 {
			mid = mid[len(left):]
		}
		prefix := ""
		if lineStartCol > 0 {
			prefix = truncateToCol(line, lineStartCol)
		}
		suffix := ""
		if lineEndCol < textWidth(line) {
			fullEnd := truncateToCol(line, lineEndCol)
			midEnd := fullEnd[len(prefix):]
			suffix = line[len(prefix)+ansi.StringWidth(midEnd):]
		}
		out[i] = prefix + selectionStyle.Render(mid) + suffix
	}
	return out
}

func textWidth(s string) int {
	return ansi.StringWidth(s)
}

func truncateToCol(s string, col int) string {
	w := 0
	lastBoundary := 0
	i := 0
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == 0x1b {
			escEnd := i + size
			for escEnd < len(s) {
				c := s[escEnd]
				if c == 'm' || c == 'K' || c == 'H' || c == 'J' || c == 'A' || c == 'B' || c == 'C' || c == 'D' || c == '0' {
					escEnd++
					break
				}
				escEnd++
			}
			i = escEnd
			continue
		}
		rw := ansi.StringWidth(string(r))
		if w+rw > col {
			return s[:lastBoundary]
		}
		w += rw
		i += size
		lastBoundary = i
		if w >= col {
			return s[:lastBoundary]
		}
	}
	return s
}

var selectionStyle = lipgloss.NewStyle().Reverse(true)

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
	return len(renderMsg(g, c.viewport.Width()))
}

func (c *chatModel) IsFollowing() bool {
	return c.following
}

func (c *chatModel) DragEdgeState() dragEdge {
	return c.dragEdgeFromLastSelection
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
	Duration  time.Duration
	Usage     *msgUsage
	PromptNum int
	Collapsed bool
}

func toChatMsg(c ChatMsg) chatMsg {
	return chatMsg{
		role:      c.Role,
		text:      c.Text,
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

func (t ChatModelT) BeginSelectionForTest(msgIdx, col int) {
	t.model.BeginSelection(msgIdx, col)
}

func (t ChatModelT) ExtendSelectionForTest(msgIdx, col int) {
	t.model.ExtendSelection(msgIdx, col)
}

func (t ChatModelT) EndSelectionForTest() string {
	return t.model.EndSelection()
}

func (t ChatModelT) HitTestForTest(x, y int) (msgIdx, col int) {
	return t.model.HitTest(x, y)
}

func (t ChatModelT) HitTestForDragForTest(x, y int) (msgIdx, col int) {
	return t.model.HitTestForDrag(x, y)
}

func (t ChatModelT) SelectAllForTest() {
	t.model.SelectAll()
}

func (t ChatModelT) DragEdgeForCoordForTest(y int) string {
	e := t.model.DragEdgeForCoord(y)
	switch e {
	case edgeTop:
		return "top"
	case edgeBottom:
		return "bottom"
	default:
		return "none"
	}
}

func (t ChatModelT) SubmitForTest2(text string) (chatMsg, bool) {
	return t.model.submitTest(text)
}

func (c *chatModel) submitTest(text string) (chatMsg, bool) {
	c.submit(text)
	if len(c.messages) == 0 {
		return chatMsg{}, false
	}
	return c.messages[len(c.messages)-1], true
}

func (t ChatModelT) SetSizeForTest(w, h int) {
	t.model.SetSize(w, h)
}

func (t ChatModelT) AtTopForTest() bool {
	return t.model.AtTop()
}

func (t ChatModelT) AtBottomForTest() bool {
	return t.model.AtBottom()
}

func (t ChatModelT) ScrollUpForTest(n int) {
	t.model.ScrollUp(n)
}

func NewSpinnerForTest() SpinnerT {
	return SpinnerT{model: newSpinner()}
}

type SpinnerT struct {
	model spinner.Model
}

func (s SpinnerT) View() string {
	return s.model.View()
}

func (s SpinnerT) Update(msg tea.Msg) (SpinnerT, tea.Cmd) {
	out, cmd := s.model.Update(msg)
	return SpinnerT{model: out}, cmd
}

type SpinnerTickType = spinner.TickMsg

func (s SpinnerT) TickForTest() SpinnerTickType {
	return s.model.Tick().(SpinnerTickType)
}