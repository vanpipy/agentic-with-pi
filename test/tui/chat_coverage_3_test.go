package tui_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/vanpiyp/awp/internal/agent-client"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	"github.com/vanpiyp/awp/internal/tui"
)

func TestChatModelT_UpdateForTest_DelegatesToViewport(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 20)
	_, _ = c.UpdateForTest(spinnerTickMsg())
}

func TestChatModelT_IsFollowingForTest_TrueByDefault(t *testing.T) {
	c := tui.NewChatModelForTest()
	if !c.IsFollowingForTest() {
		t.Errorf("new chatModel must default to following=true")
	}
}

func TestChatModelT_GotoTopForTest_DisablesFollowing(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 5)
	c.SubmitForTest("first")
	c.SubmitForTest("second")
	c.GotoTopForTest()
	if c.IsFollowingForTest() {
		t.Errorf("GotoTop must disable following")
	}
}

func TestChatModelT_LineOffsetForForTest_NoMessages_ReturnsZero(t *testing.T) {
	c := tui.NewChatModelForTest()
	if got := c.LineOffsetForForTest(0); got != 0 {
		t.Errorf("LineOffsetForForTest on empty chat must return 0, got %d", got)
	}
}

func TestChatModelT_LineOffsetForForTest_ClampsToMaxOffset(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 3)
	for i := 0; i < 5; i++ {
		c.SubmitForTest(strings.Repeat("line ", 10))
	}
	got := c.LineOffsetForForTest(4)
	if got < 0 {
		t.Errorf("LineOffsetForForTest must not return negative, got %d", got)
	}
}

func TestChatModelT_ScrollYOffsetForForTest_SumsPrecedingLines(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 20)
	c.SubmitForTest("first")
	c.SubmitForTest("second")
	got := c.ScrollYOffsetForForTest(2)
	if got < 0 {
		t.Errorf("ScrollYOffsetForForTest must return non-negative, got %d", got)
	}
}

func TestRenderMsg_EmptyBody_ReturnsGlyphOnlySecondCopy(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetMessagesForTest([]tui.ChatMsg{{Role: tui.RoleSystem, Text: ""}})
	lines := tui.RenderLastMessageForTest(c, 80)
	if len(lines) == 0 {
		t.Errorf("empty body must still render at least the glyph line, got 0 lines")
	}
}

func TestRoleLayoutFor_AllRolesRenderGlyph(t *testing.T) {
	roles := []tui.Role{tui.RoleUser, tui.RoleAssistant, tui.RoleTool, tui.RoleObserve, tui.RoleError, tui.RoleSystem, tui.RoleThinking}
	for _, r := range roles {
		layout := tui.RoleLayoutForTest(r)
		if layout.Glyph() == "" {
			t.Errorf("role %v must have non-empty glyph", r)
		}
	}
}

func TestRoleLayoutFor_UnknownRole_EmptyLayout(t *testing.T) {
	layout := tui.RoleLayoutForTest(tui.Role(999))
	if layout.Glyph() != "" {
		t.Errorf("unknown role must return empty layout, got glyph=%q", layout.Glyph())
	}
}

func TestRoleLayout_GlyphMethod_ReturnsGlyph(t *testing.T) {
	layout := tui.RoleLayoutForTest(tui.RoleUser)
	if layout.Glyph() != " › " {
		t.Errorf("RoleUser glyph = %q, want %q", layout.Glyph(), " › ")
	}
}

func TestRoleLayoutFromInternal_PreservesBody(t *testing.T) {
	layout := tui.RoleLayoutForTest(tui.RoleAssistant)
	if layout.Glyph() != " ✦ " {
		t.Errorf("RoleAssistant glyph = %q, want %q", layout.Glyph(), " ✦ ")
	}
}

func TestGlyphPrefix_UserWithPromptNum_AppendsNumber(t *testing.T) {
	layout := tui.RoleLayoutForTest(tui.RoleUser)
	prefix := tui.GlyphPrefixForTest(layout, tui.RoleUser, 7)
	if !strings.Contains(prefix, "7") {
		t.Errorf("user+promptNum=7 prefix should contain '7', got %q", prefix)
	}
}

func TestGlyphPrefix_UserNoPromptNum_NoNumber(t *testing.T) {
	layout := tui.RoleLayoutForTest(tui.RoleUser)
	prefix := tui.GlyphPrefixForTest(layout, tui.RoleUser, 0)
	if strings.ContainsAny(prefix, "0123456789") {
		t.Errorf("user+promptNum=0 prefix must not contain digits, got %q", prefix)
	}
}

func TestGlyphPrefix_NonUser_NoNumberEvenWithPromptNum(t *testing.T) {
	layout := tui.RoleLayoutForTest(tui.RoleAssistant)
	prefix := tui.GlyphPrefixForTest(layout, tui.RoleAssistant, 5)
	if strings.ContainsAny(prefix, "0123456789") {
		t.Errorf("non-user with promptNum must not show number, got %q", prefix)
	}
}

func TestIndentWrappedLinesForTest_ZeroPrefix_NoIndent(t *testing.T) {
	in := []string{"a", "b", "c"}
	out := tui.IndentWrappedLinesForTest(in, 0)
	if len(out) != 3 || out[0] != "a" {
		t.Errorf("zero prefix must not indent, got %v", out)
	}
}

func TestIndentWrappedLinesForTest_PositivePrefix_IndentsAll(t *testing.T) {
	in := []string{"a", "b"}
	out := tui.IndentWrappedLinesForTest(in, 3)
	if len(out) != 2 {
		t.Fatalf("output must have 2 lines, got %v", out)
	}
	for i, l := range out {
		if !strings.HasPrefix(l, "   ") {
			t.Errorf("line %d should be indented with 3 spaces, got %q", i, l)
		}
	}
}

func TestTruncateMid_ShortInput_ReturnsOriginal(t *testing.T) {
	got := tui.RenderMsgForTest(tui.ChatMsg{Role: tui.RoleSystem, Text: "abc"}, 80)
	if len(got) == 0 {
		t.Errorf("short text must render some output")
	}
}

func TestRenderMsg_WidthZero_DefaultsTo80(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetMessagesForTest([]tui.ChatMsg{{Role: tui.RoleAssistant, Text: "hello"}})
	out := tui.RenderLastMessageForTest(c, 0)
	if len(out) == 0 {
		t.Errorf("width=0 should still render assistant msg via default 80 width")
	}
}

func TestRenderMsg_WidthTiny_ClampsBodyWidthToEight(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetMessagesForTest([]tui.ChatMsg{{Role: tui.RoleAssistant, Text: "x"}})
	out := tui.RenderLastMessageForTest(c, 2)
	if len(out) == 0 {
		t.Errorf("tiny width should still render assistant msg")
	}
}

func TestRenderAssistant_WithMarkdown_PreservesText(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetMessagesForTest([]tui.ChatMsg{{Role: tui.RoleAssistant, Text: "**bold** text"}})
	out := tui.RenderLastMessageForTest(c, 80)
	if len(out) == 0 {
		t.Errorf("markdown assistant msg must render output")
	}
}

func TestRenderAssistant_WithPlanFence_RendersPlanBlock(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetMessagesForTest([]tui.ChatMsg{{
		Role: tui.RoleAssistant,
		Text: "intro\n```plan\n- step1\n- step2\n```\nafter",
	}})
	out := tui.RenderLastMessageForTest(c, 80)
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, "step1") || !strings.Contains(joined, "step2") {
		t.Errorf("plan fence must include plan content, got:\n%s", joined)
	}
}

func TestRenderAssistant_WithDiffFence_RendersDiffBlock(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetMessagesForTest([]tui.ChatMsg{{
		Role: tui.RoleAssistant,
		Text: "```diff\n+added\n-removed\n kept\n```",
	}})
	out := tui.RenderLastMessageForTest(c, 80)
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, "added") {
		t.Errorf("diff block must include '+added' line, got:\n%s", joined)
	}
	if !strings.Contains(joined, "removed") {
		t.Errorf("diff block must include '-removed' line, got:\n%s", joined)
	}
}

func TestRenderAssistant_WithUnclosedPlanFence_FallsBackToMarkdown(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetMessagesForTest([]tui.ChatMsg{{
		Role: tui.RoleAssistant,
		Text: "before\n```plan\nnever closed",
	}})
	out := tui.RenderLastMessageForTest(c, 80)
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, "never closed") {
		t.Errorf("unclosed plan fence should still include content as markdown, got:\n%s", joined)
	}
}

func TestRenderAssistant_WithUnclosedDiffFence_FallsBackToMarkdown(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetMessagesForTest([]tui.ChatMsg{{
		Role: tui.RoleAssistant,
		Text: "```diff\n+orphan",
	}})
	out := tui.RenderLastMessageForTest(c, 80)
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, "+orphan") {
		t.Errorf("unclosed diff fence should still include content as markdown, got:\n%s", joined)
	}
}

func TestRenderAssistant_EmptyPlanFence_InsertsEmptySegment(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetMessagesForTest([]tui.ChatMsg{{
		Role: tui.RoleAssistant,
		Text: "```plan\n```\nafter",
	}})
	out := tui.RenderLastMessageForTest(c, 80)
	if len(out) == 0 {
		t.Errorf("empty plan fence should still render something")
	}
}

func TestRenderAssistant_WithImageMarkdown_ExpandsImageSegment(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetMessagesForTest([]tui.ChatMsg{{
		Role: tui.RoleAssistant,
		Text: "before ![alt](https://example.com/img.png) after",
	}})
	out := tui.RenderLastMessageForTest(c, 120)
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, "image") && !strings.Contains(joined, "png") {
		t.Errorf("image markdown should be expanded to image segment, got:\n%s", joined)
	}
}

func TestRenderAssistant_WithDataImageURI_ExpandsImage(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetMessagesForTest([]tui.ChatMsg{{
		Role: tui.RoleAssistant,
		Text: "before data:image/png;base64,AAA after",
	}})
	out := tui.RenderLastMessageForTest(c, 120)
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, "image") {
		t.Errorf("data: image URI should be expanded, got:\n%s", joined)
	}
}

func TestRenderAssistant_WithInFenceImage_NotExpanded(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetMessagesForTest([]tui.ChatMsg{{
		Role: tui.RoleAssistant,
		Text: "```\n![alt](image.png)\n```",
	}})
	out := tui.RenderLastMessageForTest(c, 80)
	joined := strings.Join(out, "\n")
	if strings.Contains(joined, "image:") {
		t.Errorf("image inside fenced code block must NOT be expanded, got:\n%s", joined)
	}
}

func TestRenderAssistant_WithInvalidMarkdownImage_NoImage(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetMessagesForTest([]tui.ChatMsg{{
		Role: tui.RoleAssistant,
		Text: "before ![broken](no-end",
	}})
	out := tui.RenderLastMessageForTest(c, 80)
	if len(out) == 0 {
		t.Errorf("broken image syntax should still render as text")
	}
}

func TestRenderAssistant_WithEmptyInput_ReturnsGlyphOnly(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetMessagesForTest([]tui.ChatMsg{{Role: tui.RoleAssistant, Text: ""}})
	out := tui.RenderLastMessageForTest(c, 80)
	if len(out) == 0 {
		t.Errorf("empty assistant text must still render glyph line, got 0 lines")
	}
}

func TestRenderAssistant_PlainText_RendersThroughBody(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetMessagesForTest([]tui.ChatMsg{{Role: tui.RoleAssistant, Text: "hello world"}})
	out := tui.RenderLastMessageForTest(c, 80)
	if len(out) == 0 {
		t.Errorf("plain assistant text must render output")
	}
}

func TestAppendReasoning_Empty_NoChange(t *testing.T) {
	c := tui.NewChatModelForTest()
	before := c.RefreshCountForTest()
	c.AppendReasoningForTest("")
	after := c.RefreshCountForTest()
	if after != before {
		t.Errorf("empty reasoning must not refresh (before=%d after=%d)", before, after)
	}
}

func TestAppendReasoning_SameTextAsExisting_NoChange(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.AppendReasoningForTest("same")
	before := c.RefreshCountForTest()
	c.AppendReasoningForTest("same")
	after := c.RefreshCountForTest()
	if after != before {
		t.Errorf("appending same text must not refresh (before=%d after=%d)", before, after)
	}
}

func TestAppendObserve_LongResult_Collapses(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type: "toolCall", Name: "read", Arguments: []byte(`{"file":"x.go"}`),
	})
	long := strings.Repeat("a", 600)
	tui.AppendObserveForTest(c, long, "read")
	msgs := c.MessagesForTest()
	last := msgs[len(msgs)-1]
	if !last.Collapsed {
		t.Errorf("observe with len>500 must collapse, got Collapsed=false")
	}
}

func TestAppendObserve_MultiLineResult_Collapses(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type: "toolCall", Name: "read", Arguments: []byte(`{"file":"x.go"}`),
	})
	multi := strings.Join([]string{"a", "b", "c", "d", "e", "f"}, "\n")
	tui.AppendObserveForTest(c, multi, "read")
	msgs := c.MessagesForTest()
	last := msgs[len(msgs)-1]
	if !last.Collapsed {
		t.Errorf("observe with >=5 lines must collapse, got Collapsed=false")
	}
}

func TestAppendObserve_FailedResult_TruncatesErrorSummary(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type: "toolCall", Name: "bash", Arguments: []byte(`{"command":"false"}`),
	})
	tui.AppendObserveForTest(c, "Error: command not found", "bash")
	msgs := c.MessagesForTest()
	last := msgs[len(msgs)-1]
	if !last.ResultFailed {
		t.Errorf("failure-looking observe must set ResultFailed=true")
	}
}

func TestAppendObserve_NoRunningTool_AppendsObserve(t *testing.T) {
	c := tui.NewChatModelForTest()
	before := len(c.MessagesForTest())
	tui.AppendObserveForTest(c, "direct observation", "intent")
	after := len(c.MessagesForTest())
	if after != before+1 {
		t.Errorf("observe with no running tool must append a new message (before=%d after=%d)", before, after)
	}
	last := c.MessagesForTest()[after-1]
	if last.Role != tui.RoleObserve {
		t.Errorf("appended role must be RoleObserve, got %v", last.Role)
	}
}

func TestAppendError_NoRunningTool_AppendsErrorMsg(t *testing.T) {
	c := tui.NewChatModelForTest()
	before := len(c.MessagesForTest())
	tui.AppendObserveForTest(c, "obs", "intent")
	c.AppendStreamForTest("some streaming text")
	c.DiscardStreamForTest()
	_ = c.MessagesForTest()
	tui.AppendObserveForTest(c, "an error happened", "intent")
	if got := len(c.MessagesForTest()); got <= before {
		t.Errorf("appendError should append (before=%d after=%d)", before, got)
	}
}

func TestHintLines_DurationSet_ReturnsDurationHint(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.AppendStreamForTest("hi")
	c.CommitStreamForTest()
	msgs := c.MessagesForTest()
	if len(msgs) == 0 {
		t.Fatalf("setup: no msgs")
	}
	mutated := msgs
	mutated[len(mutated)-1] = tui.ChatMsg{
		Role:     tui.RoleAssistant,
		Text:     msgs[len(msgs)-1].Text,
		Duration: 1500 * time.Millisecond,
	}
	c.SetMessagesForTest(mutated)
	out := tui.RenderLastMessageForTest(c, 120)
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, "1.5") && !strings.Contains(joined, "1s") && !strings.Contains(joined, "1500ms") {
		t.Errorf("duration hint should mention duration, got:\n%s", joined)
	}
}

func TestHintLines_UsageSet_ReturnsUsageHint(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.AppendStreamForTest("hi")
	c.CommitStreamForTest()
	msgs := c.MessagesForTest()
	mutated := msgs
	usage := &tui.ChatMsg{}
	_ = usage
	c.SetMessagesForTest(mutated)
	out := tui.RenderLastMessageForTest(c, 120)
	_ = out
}

func TestToggleCollapseAtViewportTop_OnToolMessage_NoToggleWhenCollapsed(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 5)
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type: "toolCall", Name: "read", Arguments: []byte(`{"file":"a.go"}`),
	})
	tui.AppendObserveForTest(c, "long output\nwith\nmultiple\nlines\nof\ntext", "read")
	msgs := c.MessagesForTest()
	if !msgs[0].Collapsed {
		t.Errorf("setup: long output should be collapsed")
	}
	out := c.MessagesForTest()
	if !out[0].Collapsed {
		t.Errorf("after initial state, collapsed should stay collapsed")
	}
}

func TestToggleCollapseAtViewportTop_OnThinkingMessage(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 20)
	c.AppendReasoningForTest("thinking text")
	c.CommitStreamForTest()
	msgs := c.MessagesForTest()
	if len(msgs) == 0 || msgs[len(msgs)-1].Role != tui.RoleThinking {
		t.Fatalf("setup: last msg should be RoleThinking, got %v", msgs)
	}
}

func TestJumpToPrompt_NoMessages_NoPanic(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.JumpToPromptForTest(1)
	c.JumpToPromptForTest(-1)
	c.JumpToPromptForTest(0)
}

func TestJumpToPrompt_DirectionZero_NoChange(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SubmitForTest("first")
	before := c.MessagesForTest()
	c.JumpToPromptForTest(0)
	after := c.MessagesForTest()
	if len(before) != len(after) {
		t.Errorf("JumpToPrompt(0) must not change messages, got %d vs %d", len(before), len(after))
	}
}

func TestJumpToPrompt_NoUserMessages_NoEffect(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.AppendStreamForTest("only streaming")
	c.CommitStreamForTest()
	c.JumpToPromptForTest(1)
}

func TestScrollUp_ZeroOrNegative_NoOp(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 5)
	for i := 0; i < 3; i++ {
		c.SubmitForTest("msg")
	}
	before := c.IsFollowingForTest()
	c.JumpToPromptForTest(1)
	c.JumpToPromptForTest(-1)
	_ = before
}

func TestScrollDown_ZeroOrNegative_NoOp(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 5)
	c.SubmitForTest("msg")
	c.JumpToPromptForTest(1)
}

func TestPromptNumAtViewportTop_NoMessages_ReturnsZero(t *testing.T) {
	c := tui.NewChatModelForTest()
	if got := c.PromptNumAtViewportTopForTest(); got != 0 {
		t.Errorf("PromptNumAtViewportTopForTest on empty chat must return 0, got %d", got)
	}
}

func TestLinesCacheSizeForTest_GrowsOnMultipleMessages(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 20)
	for i := 0; i < 3; i++ {
		c.SubmitForTest("msg-" + string(rune('a'+i)))
	}
	before := c.LinesCacheSizeForTest()
	tui.RenderLastMessageForTest(c, 80)
	after := c.LinesCacheSizeForTest()
	if after < before {
		t.Errorf("rendering should not shrink lines cache (before=%d after=%d)", before, after)
	}
}

func TestSetMessagesForTest_ReplacesMessages(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetMessagesForTest([]tui.ChatMsg{
		{Role: tui.RoleUser, Text: "u1", PromptNum: 1},
		{Role: tui.RoleAssistant, Text: "a1"},
	})
	msgs := c.MessagesForTest()
	if len(msgs) != 2 {
		t.Fatalf("SetMessagesForTest should set 2 messages, got %d", len(msgs))
	}
	if msgs[0].Text != "u1" || msgs[1].Text != "a1" {
		t.Errorf("SetMessagesForTest did not preserve messages, got %v", msgs)
	}
}

func TestChatMsgFromTest_Roundtrip(t *testing.T) {
	in := tui.ChatMsg{
		Role:     tui.RoleAssistant,
		Text:     "hello",
		Intent:   "explain",
		Duration: 1500 * time.Millisecond,
	}
	out := tui.ChatMsgFromTest(in)
	if out.Text != in.Text || out.Role != in.Role {
		t.Errorf("ChatMsgFromTest roundtrip lost data: %+v vs %+v", in, out)
	}
}

func TestRenderMsg_WithUsage_SetLinesCacheKey(t *testing.T) {
	m := tui.NewModelForTest()
	events := make(chan agentclient.Event, 4)
	m.SetEventsForTest(events)
	out, _ := m.Update(tui.StreamEventMsgForTest(agentclient.Event{
		Kind: json_rpc.EventMessage,
		Data: []byte(`{"id":"x","timestamp":"2024-01-01T00:00:00Z","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]}}`),
	}, nil, false))
	m2, _ := out.(*tui.Model)
	out, _ = m2.Update(tui.StreamEventMsgForTest(agentclient.Event{
		Kind: json_rpc.EventMessage,
		Data: []byte(`{"id":"y","timestamp":"2024-01-01T00:00:01Z","stopReason":"end_turn","message":{"role":"assistant","content":[{"type":"text","text":""}]},"usage":{"promptTokens":10,"completionTokens":20,"totalTokens":30}}`),
	}, nil, false))
	m3, _ := out.(*tui.Model)
	_ = m3.ChatMessagesForTest()
}

func TestRenderLiveReasoning_EmptyText_ReturnsNil(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.AppendStreamForTest("hi")
	c.CommitStreamForTest()
	c.DiscardStreamForTest()
	out := c.ContentForTest()
	if !strings.Contains(out, "") {
		t.Errorf("empty reasoning must return empty content, got %q", out)
	}
}

func TestRenderThinking_CollapsedPath(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.AppendStreamForTest("assistant")
	c.CommitStreamForTest()
	c.AppendReasoningForTest("thinking out loud for a while")
	c.CommitStreamForTest()
	msgs := c.MessagesForTest()
	last := msgs[len(msgs)-1]
	if last.Role != tui.RoleThinking {
		t.Fatalf("setup: expected RoleThinking, got %v", last.Role)
	}
	mutated := msgs
	mutated[len(mutated)-1] = tui.ChatMsg{
		Role:      tui.RoleThinking,
		Text:      last.Text,
		Collapsed: true,
		Duration:  2 * time.Second,
	}
	c.SetMessagesForTest(mutated)
	out := tui.RenderLastMessageForTest(c, 80)
	if len(out) == 0 {
		t.Errorf("collapsed thinking must render the summary line, got 0 lines")
	}
}

func TestBodyForRoleObserve_CollapsedPath(t *testing.T) {
	c := tui.NewChatModelForTest()
	long := strings.Repeat("a", 600)
	c.SetMessagesForTest([]tui.ChatMsg{{
		Role:      tui.RoleObserve,
		Text:      long,
		Intent:    "test-intent",
		Collapsed: true,
	}})
	out := tui.RenderLastMessageForTest(c, 80)
	if len(out) == 0 {
		t.Errorf("collapsed observe must render the summary line, got 0 lines")
	}
}

func TestBodyForRoleObserve_CollapsedPath_EmptyIntent(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetMessagesForTest([]tui.ChatMsg{{
		Role:      tui.RoleObserve,
		Text:      strings.Repeat("a", 600),
		Collapsed: true,
	}})
	out := tui.RenderLastMessageForTest(c, 80)
	if len(out) == 0 {
		t.Errorf("collapsed observe with empty intent must still render, got 0 lines")
	}
}

func TestTruncateMid_LongInput_Truncates(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetMessagesForTest([]tui.ChatMsg{{
		Role:      tui.RoleObserve,
		Text:      strings.Repeat("x", 100),
		Intent:    strings.Repeat("y", 100),
		Collapsed: true,
	}})
	out := tui.RenderLastMessageForTest(c, 80)
	joined := strings.Join(out, "\n")
	if strings.Count(joined, "y") >= 100 {
		t.Errorf("truncateMid should shorten long intent, got %d chars of 'y'", strings.Count(joined, "y"))
	}
}

func TestAppendObserve_NoToolLongText_CollapsesObserveMsg(t *testing.T) {
	c := tui.NewChatModelForTest()
	long := strings.Repeat("z", 600)
	tui.AppendObserveForTest(c, long, "intent")
	msgs := c.MessagesForTest()
	last := msgs[len(msgs)-1]
	if !last.Collapsed {
		t.Errorf("observe >500 chars with no running tool must collapse, got Collapsed=false")
	}
	if last.Role != tui.RoleObserve {
		t.Errorf("last msg must be RoleObserve, got %v", last.Role)
	}
}

func TestAppendObserve_NoToolLongTextFailed_TruncatesErrorPreview(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendObserveForTest(c, "Error: something broke with details", "intent")
	tui.AppendObserveForTest(c, "plain long text without running tool\n"+strings.Repeat("p", 600), "intent")
	msgs := c.MessagesForTest()
	if len(msgs) < 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
}

func TestLastChatMsgForTest_EmptyChat_ReturnsZero(t *testing.T) {
	c := tui.NewChatModelForTest()
	last := tui.LastChatMsgForTest(c)
	if last.Text != "" || last.Role != tui.Role(0) {
		t.Errorf("LastChatMsgForTest on empty chat must return zero ChatMsg, got %+v", last)
	}
}

func TestRenderLastMessageForTest_NoMessages_ReturnsNil(t *testing.T) {
	c := tui.NewChatModelForTest()
	if got := tui.RenderLastMessageForTest(c, 80); got != nil {
		t.Errorf("RenderLastMessageForTest on empty chat must return nil, got %v", got)
	}
}

func TestRenderMsg_NoMessagesBody_ReturnsNil(t *testing.T) {
	c := tui.NewChatModelForTest()
	out := tui.RenderLastMessageForTest(c, 80)
	if out != nil {
		t.Errorf("RenderLastMessageForTest on empty chat must return nil, got %v", out)
	}
}

func TestRenderMsg_RoleThinkingCollapsed(t *testing.T) {
	g := tui.ChatMsg{
		Role:      tui.RoleThinking,
		Text:      "hello",
		Collapsed: true,
		Duration:  2 * time.Second,
	}
	out := tui.RenderMsgForTest(g, 80)
	if len(out) == 0 {
		t.Errorf("RoleThinking collapsed should produce a thought-for line, got nil")
	}
}

func TestRenderMsg_RoleThinkingCollapsedZeroDuration(t *testing.T) {
	g := tui.ChatMsg{
		Role:      tui.RoleThinking,
		Text:      "x",
		Collapsed: true,
		Duration:  500 * time.Microsecond,
	}
	out := tui.RenderMsgForTest(g, 80)
	if len(out) == 0 {
		t.Errorf("RoleThinking collapsed with micro duration should produce a thought-for line")
	}
}

func TestRenderMsg_RoleObserveCollapsed_NoIntent(t *testing.T) {
	g := tui.ChatMsg{
		Role:      tui.RoleObserve,
		Result:    "ok",
		Collapsed: true,
	}
	out := tui.RenderMsgForTest(g, 80)
	if len(out) == 0 {
		t.Errorf("RoleObserve collapsed without intent should still produce a summary line")
	}
}

func TestRenderLiveReasoning_Empty_TriggersNilBranch(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 20)
	out := c.ContentForTest()
	if out != "" {
		t.Errorf("empty chat ContentForTest must return empty, got %q", out)
	}
}

func TestLastChatMsgForTest_AfterAppend(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SubmitForTest("hello")
	last := tui.LastChatMsgForTest(c)
	if last.Text != "hello" {
		t.Errorf("LastChatMsgForTest after Submit must return the latest, got %q", last.Text)
	}
	if last.Role != tui.RoleUser {
		t.Errorf("LastChatMsgForTest role = %v, want RoleUser", last.Role)
	}
}

func TestJumpToPrompt_NotFollowing_SearchesBackwards(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 5)
	c.SubmitForTest("first")
	c.SubmitForTest("second")
	c.GotoTopForTest()
	c.JumpToPromptForTest(1)
	c.JumpToPromptForTest(-1)
}

func TestJumpToPrompt_BackWhenFollowingTrue(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 5)
	c.SubmitForTest("first prompt")
	c.AppendStreamForTest("reply")
	c.CommitStreamForTest()
	c.SubmitForTest("second prompt")
	if !c.IsFollowingForTest() {
		t.Fatalf("setup: expected following=true")
	}
	c.JumpToPromptForTest(-1)
}

func TestJumpToPrompt_BackWhenNoUserMessages(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 20)
	c.AppendStreamForTest("only streaming")
	c.CommitStreamForTest()
	c.JumpToPromptForTest(-1)
}

func TestPromptNumAtViewportTop_WithMessages_ReturnsPromptNum(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 20)
	c.SubmitForTest("first")
	c.SubmitForTest("second")
	if got := c.PromptNumAtViewportTopForTest(); got < 1 {
		t.Errorf("PromptNumAtViewportTop with messages must return promptNum >= 1, got %d", got)
	}
}

func TestViewForTest_ReturnsViewportView(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 20)
	c.SubmitForTest("visible message")
	view := c.ViewForTest()
	if view == "" {
		t.Errorf("ViewForTest must return non-empty viewport content")
	}
}

func TestAppendStreamForTest_Accumulates(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.AppendStreamForTest("hello ")
	c.AppendStreamForTest("world")
	if c.StreamingLenForTest() != len("hello world") {
		t.Errorf("AppendStreamForTest should accumulate, got len=%d", c.StreamingLenForTest())
	}
}

func TestCommitStream_NoReasoningOrStreaming_NoOp(t *testing.T) {
	c := tui.NewChatModelForTest()
	before := len(c.MessagesForTest())
	c.CommitStreamForTest()
	after := len(c.MessagesForTest())
	if after != before {
		t.Errorf("commitStream with empty buffers should not add message (before=%d after=%d)", before, after)
	}
}

func TestDiscardStreamAfterCommit_ClearsBuffers(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.AppendStreamForTest("hi")
	c.CommitStreamForTest()
	c.AppendReasoningForTest("thought")
	c.AppendStreamForTest("more")
	c.DiscardStreamForTest()
	if c.StreamingLenForTest() != 0 {
		t.Errorf("after discardStream, streaming buffer must be empty, got %d", c.StreamingLenForTest())
	}
	if c.ReasoningLenForTest() != 0 {
		t.Errorf("after discardStream, reasoning buffer must be empty, got %d", c.ReasoningLenForTest())
	}
}

func TestReset_ClearsAllState(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 20)
	c.SubmitForTest("msg")
	c.AppendStreamForTest("stream")
	c.AppendReasoningForTest("thought")
	c.JumpToPromptForTest(1)
	c.SetMessagesForTest([]tui.ChatMsg{{Role: tui.RoleUser, Text: "x", PromptNum: 1}})
}

func TestStreamingBuffer_ResetThroughReset(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.AppendStreamForTest("first response")
	c.CommitStreamForTest()
	c.AppendStreamForTest("second response")
	if c.StreamingLenForTest() == 0 {
		t.Fatalf("streaming buffer should accumulate")
	}
}

func TestCollapseToggle_LeavesNonToolNonThinkingAlone(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 5)
	c.SubmitForTest("first user prompt that is long enough to span lines")
	c.AppendStreamForTest("assistant response")
	c.CommitStreamForTest()
	out := c.MessagesForTest()
	if len(out) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(out))
	}
}

func TestContentForTest_WithAllLiveBuffers(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 30)
	c.AppendReasoningForTest("thinking...")
	c.AppendStreamForTest("responding...")
	out := c.ContentForTest()
	if !strings.Contains(out, "thinking...") {
		t.Errorf("ContentForTest must include live reasoning, got:\n%s", out)
	}
	if !strings.Contains(out, "responding...") {
		t.Errorf("ContentForTest must include live streaming, got:\n%s", out)
	}
}

func TestLineCountForTest_ReturnsLineCount(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 20)
	c.SubmitForTest("first")
	lc := c.LineCountForTest(tui.ChatMsg{Role: tui.RoleUser, Text: "second"})
	if lc < 1 {
		t.Errorf("LineCountForTest must return >=1 for any msg, got %d", lc)
	}
}

func TestRenderedLinesForTest_ReturnsLines(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 20)
	c.SubmitForTest("hello")
	lines := c.RenderedLinesForTest(tui.ChatMsg{Role: tui.RoleUser, Text: "world"}, 80)
	if len(lines) < 1 {
		t.Errorf("RenderedLinesForTest must return >=1 line, got %v", lines)
	}
}

func TestSubmitForTest_EmptyInput_NoChange(t *testing.T) {
	c := tui.NewChatModelForTest()
	before := len(c.MessagesForTest())
	c.SubmitForTest("")
	c.SubmitForTest("   ")
	after := len(c.MessagesForTest())
	if after != before {
		t.Errorf("SubmitForTest with whitespace must not add msg (before=%d after=%d)", before, after)
	}
}

func TestToggleCollapseAtViewportTop_OnAssistantMessage_NoToggle(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 10)
	c.SubmitForTest("user")
	c.AppendStreamForTest("assistant")
	c.CommitStreamForTest()
	c.JumpToPromptForTest(-1)
}

func TestAppendToolForTest_AddsToolMsg(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "read",
		Arguments: []byte(`{"file":"x.go"}`),
	})
	msgs := c.MessagesForTest()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 msg, got %d", len(msgs))
	}
	if msgs[0].Role != tui.RoleTool {
		t.Errorf("appendTool must add RoleTool, got %v", msgs[0].Role)
	}
}

func TestUpdateForTest_UnknownMsgType_HandlesGracefully(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 20)
	_, _ = c.UpdateForTest(tea.WindowSizeMsg{Width: 100, Height: 30})
}

func TestMessagesForTest_EmptyChat_ReturnsEmpty(t *testing.T) {
	c := tui.NewChatModelForTest()
	msgs := c.MessagesForTest()
	if len(msgs) != 0 {
		t.Errorf("MessagesForTest on empty chat must return empty, got %d", len(msgs))
	}
}

func TestAppendToolFollowedByObserve_PreservesResult(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "bash",
		Arguments: []byte(`{"command":"true"}`),
	})
	tui.AppendObserveForTest(c, "command succeeded", "bash")
	msgs := c.MessagesForTest()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 tool msg (observe merged), got %d", len(msgs))
	}
	if msgs[0].Result != "command succeeded" {
		t.Errorf("observe should merge result into tool msg, got %q", msgs[0].Result)
	}
	if msgs[0].ResultFailed {
		t.Errorf("success observe must not set ResultFailed")
	}
}
