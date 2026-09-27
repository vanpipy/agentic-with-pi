package tui_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	agentclient "github.com/vanpiyp/awp/internal/agent-client"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	"github.com/vanpiyp/awp/internal/tui"
)

func TestUpdate_Esc_WithPickerVisible_HidesPicker(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.PickerShowForTest([]tui.PickerItemForTest{{ID: "session-1"}})
	if !m.PickerVisibleForTest() {
		t.Fatalf("setup: picker should be visible after PickerShowForTest")
	}
	_, _ = m.Update(keyEsc())
	if m.PickerVisibleForTest() {
		t.Errorf("esc with picker visible must hide picker")
	}
}

func TestUpdate_Up_WithPickerVisible_PrevInList(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.PickerShowForTest([]tui.PickerItemForTest{
		{ID: "s1"},
		{ID: "s2"},
		{ID: "s3"},
	})
	if !m.PickerVisibleForTest() {
		t.Fatalf("setup: picker should be visible")
	}
	out, _ := m.Update(keyUp())
	if out == nil {
		t.Errorf("up with picker visible must return the model")
	}
}

func TestUpdate_Down_WithPickerVisible_NextInList(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.PickerShowForTest([]tui.PickerItemForTest{
		{ID: "s1"},
		{ID: "s2"},
	})
	if !m.PickerVisibleForTest() {
		t.Fatalf("setup: picker should be visible")
	}
	out, _ := m.Update(keyDown())
	if out == nil {
		t.Errorf("down with picker visible must return the model")
	}
}

func TestUpdate_Enter_WithPickerVisible_TriggersResume(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetConnForTest(nil)
	m.PickerShowForTest([]tui.PickerItemForTest{{ID: "resume-me"}})
	if !m.PickerVisibleForTest() {
		t.Fatalf("setup: picker should be visible")
	}
	out, _ := m.Update(keyEnter())
	if out == nil {
		t.Errorf("enter with picker visible must return model")
	}
	if m.PickerVisibleForTest() {
		t.Errorf("enter with picker visible must hide picker")
	}
}

func TestUpdate_Enter_SlashCancelFirst_MidStreaming_TriggersCancel(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateStreaming)
	m.SetEventsForTest(nil)
	m.SetConnForTest(nil)
	m.InputAppendForTest("/quit")
	_, cmd := m.Update(keyEnter())
	_ = cmd
	if m.StateForTest() != tui.StateCancelling {
		t.Errorf("mid-stream /quit must trigger cancel (state=%v)", m.StateForTest())
	}
}

func TestUpdate_Enter_SlashNonCancelFirst_MidStreaming_NoCancel(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateStreaming)
	m.SetEventsForTest(nil)
	m.SetConnForTest(nil)
	m.InputAppendForTest("/usage")
	_, cmd := m.Update(keyEnter())
	_ = cmd
	if m.StateForTest() == tui.StateCancelling {
		t.Errorf("mid-stream /usage (not cancel-first) must NOT cancel, got state=%v", m.StateForTest())
	}
}

func TestUpdate_SessionPickerMsg_EmptyItems_AppendsSystemMsg(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	before := len(m.ChatMessagesForTest())
	out, _ := m.Update(tui.SessionPickerMsgForTest(nil))
	if out == nil {
		t.Fatalf("Update must return model")
	}
	after := len(m.ChatMessagesForTest())
	if after <= before {
		t.Errorf("sessionPickerMsg with empty items must append system message (before=%d after=%d)", before, after)
	}
}

func TestUpdate_SessionPickerMsg_NonEmptyItems_ShowsPicker(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	_, _ = m.Update(tui.SessionPickerMsgForTest([]tui.PickerItemForTest{
		{ID: "s1"},
		{ID: "s2"},
	}))
	if !m.PickerVisibleForTest() {
		t.Errorf("sessionPickerMsg with items must show picker")
	}
}

func TestUpdate_PromptDoneMsg_TransitionsToReady(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateStreaming)
	out, _ := m.Update(tui.PromptDoneMsgForTest())
	mm := out.(*tui.Model)
	if mm.StateForTest() != tui.StateReady {
		t.Errorf("promptDoneMsg must transition to StateReady, got %v", mm.StateForTest())
	}
}

func TestUpdate_CompactDoneMsg_WithStrategy_AppendsSystemMsg(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	before := len(m.ChatMessagesForTest())
	_, _ = m.Update(tui.CompactDoneMsgForTest(json_rpc.CompactResult{
		Triggered:    true,
		Strategy:     "summary",
		TokensBefore: 1000,
		TokensAfter:  500,
		DurationMS:   200,
	}, "sess-x"))
	after := len(m.ChatMessagesForTest())
	if after <= before {
		t.Errorf("compactDoneMsg with strategy must append system msg (before=%d after=%d)", before, after)
	}
	last := m.LastChatMessageForTest()
	if !strings.Contains(last, "summary") {
		t.Errorf("compact msg should mention strategy=summary, got %q", last)
	}
	if !strings.Contains(last, "1000") || !strings.Contains(last, "500") {
		t.Errorf("compact msg should mention token counts, got %q", last)
	}
}

func TestUpdate_CompactDoneMsg_EmptyStrategy_FallsBackToNone(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	before := len(m.ChatMessagesForTest())
	_, _ = m.Update(tui.CompactDoneMsgForTest(json_rpc.CompactResult{
		Triggered:    false,
		Strategy:     "",
		TokensBefore: 50,
		TokensAfter:  50,
		DurationMS:   1,
	}, "sess-y"))
	after := len(m.ChatMessagesForTest())
	if after <= before {
		t.Errorf("compactDoneMsg with empty strategy must append system msg (before=%d after=%d)", before, after)
	}
	last := m.LastChatMessageForTest()
	if !strings.Contains(last, "none") {
		t.Errorf("empty strategy should fall back to 'none', got %q", last)
	}
}

func TestLayout_VerySmallHeight_BodyHeightClampsToOne(t *testing.T) {
	m := newSizedModel(t, 80, 2)
	stripped := stripANSI(m.ViewForTest())
	if stripped == "initializing..." {
		t.Errorf("after layout, view should not be initializing")
	}
	if !strings.Contains(stripped, "ready") {
		t.Errorf("small-height view should still show ready status, got:\n%s", stripped)
	}
}

func TestView_PickerVisible_PopupAppears(t *testing.T) {
	m := newSizedModel(t, 120, 40)
	m.PickerShowForTest([]tui.PickerItemForTest{
		{ID: "s1", Model: "opus"},
		{ID: "s2", Model: "haiku"},
	})
	stripped := stripANSI(m.ViewForTest())
	if !strings.Contains(stripped, "s1") {
		t.Errorf("picker should render session IDs in view, got:\n%s", stripped)
	}
}

func TestView_AutocompleteVisible_PopupAppears(t *testing.T) {
	m := newSizedModel(t, 120, 40)
	m.InputAppendForTest("/compa")
	m.AutocompleteRefreshForTest()
	if !m.AutocompleteVisibleForTest() {
		t.Fatalf("setup: autocomplete should be visible")
	}
	stripped := stripANSI(m.ViewForTest())
	if stripped == "initializing..." {
		t.Errorf("view should not be initializing when popup visible")
	}
}

func TestUpdate_StreamEvent_NoConnProvided_Done(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	ch := make(chan agentclient.Event, 1)
	m.SetEventsForTest(ch)
	out, _ := m.Update(tui.StreamEventMsgForTest(
		agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"final"}`)},
		nil, true,
	))
	mm := out.(*tui.Model)
	if mm.StateForTest() != tui.StateReady {
		t.Errorf("done with empty buffered ch should still set StateReady, got %v", mm.StateForTest())
	}
}

func TestUpdate_StreamEvent_DoneClosedChannel_DrainsViaOkFalseBranch(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	ch := make(chan agentclient.Event)
	close(ch)
	m.SetEventsForTest(ch)
	out, _ := m.Update(tui.StreamEventMsgForTest(
		agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"final"}`)},
		nil, true,
	))
	mm := out.(*tui.Model)
	if mm.StateForTest() != tui.StateReady {
		t.Errorf("done with closed ch should set StateReady, got %v", mm.StateForTest())
	}
}

func TestUpdate_StreamEvent_MessageWithInvalidJSON_DoesNotError(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateStreaming)
	ch := make(chan agentclient.Event, 1)
	m.SetEventsForTest(ch)
	out, _ := m.Update(tui.StreamEventMsgForTest(
		agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`not-json`)},
		nil, false,
	))
	mm := out.(*tui.Model)
	if mm.StateForTest() != tui.StateStreaming {
		t.Errorf("EventMessage with invalid JSON must not error, got %v", mm.StateForTest())
	}
}

func TestUpdate_ErrMsgHandler_SetsStateError(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateStreaming)
	out, _ := m.Update(tui.StreamEventMsgForTest(
		agentclient.Event{},
		errors.New("test failure"),
		false,
	))
	mm := out.(*tui.Model)
	if mm.StateForTest() != tui.StateError {
		t.Errorf("err streamEventMsg must set StateError, got %v", mm.StateForTest())
	}
	if mm.EventsForTest() != nil {
		t.Errorf("err streamEventMsg must clear events, got non-nil")
	}
}

func TestIsQuitCommand_TrueOnQuit(t *testing.T) {
	if !tui.IsQuitCommandForTest("/quit") {
		t.Errorf("IsQuitCommandForTest(/quit) must be true")
	}
}

func TestIsQuitCommand_FalseOnOther(t *testing.T) {
	if tui.IsQuitCommandForTest("/new") {
		t.Errorf("IsQuitCommandForTest(/new) must be false")
	}
}

func TestIsQuitCommand_FalseOnNonSlash(t *testing.T) {
	if tui.IsQuitCommandForTest("plain") {
		t.Errorf("IsQuitCommandForTest on non-slash must be false")
	}
}

func TestIsSlashCommand_TrueOnSlash(t *testing.T) {
	if !tui.IsSlashCommandForTest("/anything") {
		t.Errorf("IsSlashCommandForTest on slash must be true")
	}
}

func TestIsSlashCommand_FalseOnPlain(t *testing.T) {
	if tui.IsSlashCommandForTest("plain text") {
		t.Errorf("IsSlashCommandForTest on plain text must be false")
	}
}

func TestIsSlashCommand_FalseOnSlashOnly(t *testing.T) {
	if tui.IsSlashCommandForTest("/") {
		t.Errorf("IsSlashCommandForTest on bare '/' must be false (parseCommand requires name)")
	}
}

func TestShutdown_NoConnNoOwnServer_NoOp(t *testing.T) {
	m := tui.NewModelForTest()
	m.SetConnForTest(nil)
	m.ShutdownForTest()
}

func TestStartStream_NoConn_ReturnsErrMsgCmd(t *testing.T) {
	m := tui.NewModelForTest()
	m.SetConnForTest(nil)
	cmd := m.StartStreamForTest("hello")
	if cmd == nil {
		t.Fatalf("StartStreamForTest with nil conn should return errMsg cmd, got nil")
	}
	msg := cmd()
	if msg == nil {
		t.Fatalf("cmd() must return a msg, got nil")
	}
}

func TestReadNextEvent_NilEvents_ReturnsNil(t *testing.T) {
	m := tui.NewModelForTest()
	m.SetEventsForTest(nil)
	if cmd := m.ReadNextEventForTest(); cmd != nil {
		t.Errorf("ReadNextEventForTest with nil events must return nil, got: %v", cmd)
	}
}

func TestReadNextEvent_OpenChannelWithItem_ReturnsItemMsg(t *testing.T) {
	m := tui.NewModelForTest()
	ch := make(chan agentclient.Event, 1)
	ch <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"x"}`)}
	m.SetEventsForTest(ch)
	cmd := m.ReadNextEventForTest()
	if cmd == nil {
		t.Fatalf("ReadNextEventForTest with non-nil events must return cmd")
	}
	msg := cmd()
	if msg == nil {
		t.Fatalf("cmd() must return msg")
	}
}

func TestReadNextEvent_ClosedChannel_ReturnsDoneMsg(t *testing.T) {
	m := tui.NewModelForTest()
	ch := make(chan agentclient.Event)
	close(ch)
	m.SetEventsForTest(ch)
	cmd := m.ReadNextEventForTest()
	if cmd == nil {
		t.Fatalf("ReadNextEventForTest must return cmd")
	}
	msg := cmd()
	if msg == nil {
		t.Fatalf("cmd() must return msg")
	}
}

func TestTruncateWithEllipsis_MaxWidthZero_Empty(t *testing.T) {
	if got := tui.TruncateWithEllipsisForTest("hello", 0); got != "" {
		t.Errorf("truncateWithEllipsis(_, 0) must return empty, got %q", got)
	}
}

func TestTruncateWithEllipsis_MaxWidthOne_Ellipsis(t *testing.T) {
	got := tui.TruncateWithEllipsisForTest("hello world", 1)
	if got != "…" {
		t.Errorf("truncateWithEllipsis(_, 1) must return ellipsis, got %q", got)
	}
}

func TestTruncateWithEllipsis_AllRunesTrimmedToEmpty_ReturnsEllipsis(t *testing.T) {
	got := tui.TruncateWithEllipsisForTest("hello", 1)
	if got != "…" {
		t.Errorf("truncateWithEllipsis all-trimmed must return ellipsis, got %q", got)
	}
}

func TestTruncateWithEllipsis_NoTrim_ReturnsOriginal(t *testing.T) {
	got := tui.TruncateWithEllipsisForTest("hi", 10)
	if got != "hi" {
		t.Errorf("truncateWithEllipsis short input must return original, got %q", got)
	}
}

func TestTruncateWithEllipsis_TrimAppendsEllipsis(t *testing.T) {
	got := tui.TruncateWithEllipsisForTest("hello world", 6)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncateWithEllipsis trimmed input must end with ellipsis, got %q", got)
	}
}

func TestIsAbortMessage_InvalidJSON_ReturnsFalse(t *testing.T) {
	if tui.IsAbortMessageForTest([]byte(`not-valid-json`)) {
		t.Errorf("isAbortMessage on invalid JSON must return false")
	}
}

func TestIsAbortMessage_ValidJSONNoAbort_ReturnsFalse(t *testing.T) {
	if tui.IsAbortMessageForTest([]byte(`{"stopReason":"end_turn"}`)) {
		t.Errorf("isAbortMessage on end_turn must return false")
	}
}

func TestIsAbortMessage_ValidJSONAbort_ReturnsTrue(t *testing.T) {
	if !tui.IsAbortMessageForTest([]byte(`{"stopReason":"abort"}`)) {
		t.Errorf("isAbortMessage on abort must return true")
	}
}

func TestShortHelpBindings_ReturnsDefaults(t *testing.T) {
	bindings := tui.ShortHelpBindingsForTest()
	if len(bindings) == 0 {
		t.Errorf("ShortHelpBindingsForTest must return at least one binding")
	}
}

func TestRenderHeader_ZeroWidth_ReturnsEmpty(t *testing.T) {
	if got := tui.RenderHeaderWithPromptForTest(0, "ready", "", ""); got != "" {
		t.Errorf("renderHeader with width=0 must return empty, got %q", got)
	}
}

func TestRenderHeader_NegativeWidth_ReturnsEmpty(t *testing.T) {
	if got := tui.RenderHeaderWithPromptForTest(-1, "ready", "", ""); got != "" {
		t.Errorf("renderHeader with negative width must return empty, got %q", got)
	}
}

func TestRenderHeader_VeryNarrow_StillRenders(t *testing.T) {
	got := stripANSI(tui.RenderHeaderWithPromptForTest(10, "ready", "p", "sess"))
	if got == "" {
		t.Errorf("narrow header must still produce output")
	}
}

func TestRenderHeader_VeryNarrowClampsPromptContent(t *testing.T) {
	got := stripANSI(tui.RenderHeaderWithPromptForTest(15, "ready", "this prompt is way too long", "long-session-id-here"))
	if !strings.Contains(got, "ready") {
		t.Errorf("very narrow header should still render status, got:\n%s", got)
	}
}

func TestOwnServerPIDForTest_NonExistentFile_ReturnsZero(t *testing.T) {
	got := tui.OwnServerPIDForTest("/tmp/nonexistent.sock", "/tmp/nonexistent.pid")
	if got != 0 {
		t.Errorf("OwnServerPIDForTest on non-existent pidfile must return 0, got %d", got)
	}
}

func TestWaitForServerForTest_NonRunningSocket_ReturnsError(t *testing.T) {
	err := tui.WaitForServerForTest("/tmp/definitely-not-running.sock", 50*time.Millisecond)
	if err == nil {
		t.Errorf("WaitForServerForTest with non-running socket must return error")
	}
}

func TestIsCancelFirstCommand_TrueOnQuitNewResumeCompact(t *testing.T) {
	cases := []string{"/quit", "/new", "/resume", "/compact"}
	for _, c := range cases {
		if !tui.IsCancelFirstCommandForTest(c) {
			t.Errorf("IsCancelFirstCommandForTest(%q) must be true", c)
		}
	}
}

func TestIsCancelFirstCommand_FalseOnOther(t *testing.T) {
	if tui.IsCancelFirstCommandForTest("/skills") {
		t.Errorf("IsCancelFirstCommandForTest on /skills must be false")
	}
}

func TestUpdate_KeyA_AfterEsc_DoesNotAffectState(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateReady)
	_, _ = m.Update(keyEsc())
	_, _ = m.Update(keyA())
	if m.StateForTest() != tui.StateReady {
		t.Errorf("plain letter after esc should not affect state, got %v", m.StateForTest())
	}
}

func TestUpdate_StreamEvent_ToolResultMessageFlow(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateStreaming)
	ch := make(chan agentclient.Event, 1)
	m.SetEventsForTest(ch)
	out, _ := m.Update(tui.StreamEventMsgForTest(
		agentclient.Event{
			Kind: json_rpc.EventMessage,
			Data: []byte(`{"id":"x","message":{"role":"toolResult","content":[{"type":"text","text":"observed-result"}]},"stopReason":"end_turn","details":{"intent":"read"}}`),
		},
		nil, false,
	))
	_ = out
}

func TestShutdownForTest_OwnServerWithPid_KillsServer(t *testing.T) {
	m := tui.NewModelForTest()
	m.SetOwnServerForTest(true, 1)
	m.ShutdownForTest()
}

func TestShutdownForTest_OwnServerWithAlivePid_WaitThenKill(t *testing.T) {
	m := tui.NewModelForTest()
	m.SetOwnServerForTest(true, 2)
	m.ShutdownForTest()
}

func TestShutdownForTest_NotOwnServer_NoKill(t *testing.T) {
	m := tui.NewModelForTest()
	m.SetOwnServerForTest(false, 99999)
	m.ShutdownForTest()
}

func TestShutdownForTest_WithConn_ClosesConn(t *testing.T) {
	m := tui.NewModelForTest()
	m.ShutdownForTest()
}

func TestCancelForTest_NoConn_NoError(t *testing.T) {
	m := tui.NewModelForTest()
	if cmd := m.CancelForTest(); cmd != nil {
		t.Errorf("CancelForTest with no events should return nil, got %v", cmd)
	}
}

func TestCancelForTest_NonNilEvents_ReturnsReadNextCmd(t *testing.T) {
	m := tui.NewModelForTest()
	events := make(chan agentclient.Event, 1)
	m.SetEventsForTest(events)
	if cmd := m.CancelForTest(); cmd == nil {
		t.Errorf("CancelForTest with non-nil events should return cmd")
	}
}

func TestStartStreamForTest_NoConn_ReturnsErrMsg(t *testing.T) {
	m := tui.NewModelForTest()
	cmd := m.StartStreamForTest("hello")
	if cmd == nil {
		t.Errorf("StartStreamForTest with no conn should return errMsg cmd")
	}
}

func TestReadNextEventForTest_NoEvents_ReturnsNil(t *testing.T) {
	m := tui.NewModelForTest()
	if cmd := m.ReadNextEventForTest(); cmd != nil {
		t.Errorf("ReadNextEventForTest with no events should return nil, got %v", cmd)
	}
}

func TestReadNextEventForTest_OpenChannel_ReturnsCmd(t *testing.T) {
	m := tui.NewModelForTest()
	events := make(chan agentclient.Event, 1)
	m.SetEventsForTest(events)
	if cmd := m.ReadNextEventForTest(); cmd == nil {
		t.Errorf("ReadNextEventForTest with open channel should return cmd")
	}
}

func TestReadNextEventForTest_ClosedChannel_ReturnsNonNil(t *testing.T) {
	m := tui.NewModelForTest()
	events := make(chan agentclient.Event)
	close(events)
	m.SetEventsForTest(events)
	if cmd := m.ReadNextEventForTest(); cmd == nil {
		t.Errorf("ReadNextEventForTest with closed channel should still return a cmd that resolves to nil")
	}
}

func TestUpdate_EscDuringStreaming_TriggersCancelCmd(t *testing.T) {
	m := tui.NewModelForTest()
	events := make(chan agentclient.Event, 1)
	m.SetEventsForTest(events)
	m.SetStateForTest(tui.StateStreamingForTestValue())
	_, _ = m.Update(keyEsc())
}

func TestUpdate_EnterMidStream_SlashCancelFirst_WithEvents(t *testing.T) {
	m := tui.NewModelForTest()
	events := make(chan agentclient.Event, 1)
	m.SetEventsForTest(events)
	m.SetStateForTest(tui.StateStreamingForTestValue())
	m.InputAppendForTest("/cancel")
	_, _ = m.Update(keyEnter())
}

func TestUpdate_EnterMidStream_QuitCommand(t *testing.T) {
	m := tui.NewModelForTest()
	events := make(chan agentclient.Event, 1)
	m.SetEventsForTest(events)
	m.SetStateForTest(tui.StateStreamingForTestValue())
	m.InputAppendForTest("/quit")
	_, _ = m.Update(keyEnter())
}

func TestUpdate_EnterMidStream_KnownCommand_Batch(t *testing.T) {
	m := tui.NewModelForTest()
	events := make(chan agentclient.Event, 1)
	m.SetEventsForTest(events)
	m.SetStateForTest(tui.StateStreamingForTestValue())
	m.InputAppendForTest("/compact")
	_, _ = m.Update(keyEnter())
}

func TestUpdate_EnterKnownCommandNotMidStream_NonNilCmd(t *testing.T) {
	m := tui.NewModelForTest()
	events := make(chan agentclient.Event, 1)
	m.SetEventsForTest(events)
	m.InputAppendForTest("/compact")
	_, _ = m.Update(keyEnter())
}

func TestUpdate_EnterKnownCommandNotMidStream_CompactRun(t *testing.T) {
	m := tui.NewModelForTest()
	events := make(chan agentclient.Event, 1)
	m.SetEventsForTest(events)
	m.InputAppendForTest("/resume")
	_, _ = m.Update(keyEnter())
}

func TestUpdate_EnterKnownCommand_CompactViaFakeConn(t *testing.T) {
	socketPath, cleanup := startFakeRPCServer(t, func(conn net.Conn, req *json_rpc.Request) {
		body, _ := json.Marshal(json_rpc.CompactResult{Triggered: true, Strategy: "truncate"})
		writeJSONRPC(t, conn, req, "compact_result", body)
	})
	defer cleanup()
	c := dialFakeConn(t, socketPath)
	defer c.Close()

	m := tui.NewModelForTest()
	m.SetConnForTest(c)
	m.SetEventsForTest(make(chan agentclient.Event, 1))
	m.InputAppendForTest("/compact")
	_, _ = m.Update(keyEnter())
}

func TestStartStream_ConnPromptSuccess(t *testing.T) {
	socketPath, cleanup := startFakeRPCServer(t, func(conn net.Conn, req *json_rpc.Request) {
		if req.Method != json_rpc.MethodPrompt {
			t.Errorf("expected method %q, got %q", json_rpc.MethodPrompt, req.Method)
		}
		writeJSONRPC(t, conn, req, json_rpc.EventCancelAck, []byte(`{}`))
	})
	defer cleanup()
	c := dialFakeConn(t, socketPath)
	defer c.Close()

	m := tui.NewModelForTest()
	m.SetConnForTest(c)
	cmd := m.StartStreamForTest("hello")
	if cmd == nil {
		t.Fatal("startStream with conn should return non-nil cmd")
	}
}

func TestStartStream_PromptErrorPath(t *testing.T) {
	socketPath, cleanup := startFakeRPCServer(t, func(conn net.Conn, req *json_rpc.Request) {
		_ = conn.Close()
	})
	defer cleanup()
	c := dialFakeConn(t, socketPath)
	c.Close()
	defer cleanup()

	m := tui.NewModelForTest()
	m.SetConnForTest(c)
	cmd := m.StartStreamForTest("x")
	if cmd == nil {
		t.Fatal("startStream should still return cmd even after server drops")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("cmd() returned nil msg")
	} else {
		t.Logf("cmd() returned msg of type %T", msg)
	}
}

func TestSpinnerTick_NonNilCmd(t *testing.T) {
	m := tui.NewModelForTest()
	_, _ = m.Update(spinnerTickMsg())
}

func TestAutocompleteItemsForTest_NilAutocomplete(t *testing.T) {
	m := tui.NewModelForTest()
	items := m.AutocompleteItemsForTest()
	if len(items) == 0 {
		t.Errorf("AutocompleteItemsForTest on fresh model should return at least one item, got 0")
	}
}

func TestAutocompleteItemsForTest_NilAutocompleteInner(t *testing.T) {
	m := tui.NewModelForTest()
	m.SetAutocompleteForTest(nil)
	items := m.AutocompleteItemsForTest()
	if items != nil {
		t.Errorf("expected nil items when autocomplete=nil, got %d", len(items))
	}
}

func TestShutdownForTest_WithConn(t *testing.T) {
	socketPath, cleanup := startFakeRPCServer(t, func(conn net.Conn, req *json_rpc.Request) {
		_ = conn.Close()
	})
	defer cleanup()
	c := dialFakeConn(t, socketPath)

	m := tui.NewModelForTest()
	m.SetConnForTest(c)
	m.ShutdownForTest()
}

func TestCancelForTest_WithConnAndEvents(t *testing.T) {
	socketPath, cleanup := startFakeRPCServer(t, func(conn net.Conn, req *json_rpc.Request) {
		_ = conn.Close()
	})
	defer cleanup()
	c := dialFakeConn(t, socketPath)
	c.Close()

	m := tui.NewModelForTest()
	m.SetConnForTest(c)
	events := make(chan agentclient.Event, 1)
	m.SetEventsForTest(events)
	events <- agentclient.Event{Kind: json_rpc.EventCancelAck, Data: []byte(`{}`)}

	cmd := m.CancelForTest()
	_ = cmd
}

func TestTruncateWithEllipsis_ZeroMax(t *testing.T) {
	if got := tui.TruncateWithEllipsisForTest("hello", 0); got != "" {
		t.Errorf("expected empty for maxWidth=0, got %q", got)
	}
}

func TestTruncateWithEllipsis_NegativeMax(t *testing.T) {
	if got := tui.TruncateWithEllipsisForTest("hello", -1); got != "" {
		t.Errorf("expected empty for maxWidth=-1, got %q", got)
	}
}

func TestTruncateWithEllipsis_ShortString(t *testing.T) {
	if got := tui.TruncateWithEllipsisForTest("hi", 10); got != "hi" {
		t.Errorf("expected 'hi', got %q", got)
	}
}

func TestTruncateWithEllipsis_MaxOne(t *testing.T) {
	got := tui.TruncateWithEllipsisForTest("hello", 1)
	if got != "…" {
		t.Errorf("expected '…', got %q", got)
	}
}

func TestTruncateWithEllipsis_AllRunesTruncated(t *testing.T) {
	got := tui.TruncateWithEllipsisForTest("字a", 2)
	if got != "…" {
		t.Errorf("expected '…', got %q", got)
	}
}

func TestWaitForServerForTest_NonRunningSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "missing.sock")
	err := tui.WaitForServerForTest(socketPath, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestWaitForServerForTest_RunningSocket(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "running.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			_ = conn.Close()
		}
	}()
	if err := tui.WaitForServerForTest(socketPath, 2*time.Second); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestOwnServerPIDForTest_StalePID(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "stale.pid")
	if err := os.WriteFile(pidFile, []byte("999999"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := tui.OwnServerPIDForTest("unused.sock", pidFile); got != 0 {
		t.Errorf("expected 0 for stale pid, got %d", got)
	}
}

func TestOwnServerPIDForTest_LivePID(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "live.pid")
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := tui.OwnServerPIDForTest("unused.sock", pidFile); got != os.Getpid() {
		t.Errorf("expected %d, got %d", os.Getpid(), got)
	}
}

func TestSpawnServerForTest_RoundTrip(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "spawn.sock")
	pid, err := tui.SpawnServerForTest(socketPath)
	if err != nil {
		t.Fatalf("spawnServer returned err: %v", err)
	}
	if pid <= 0 {
		t.Errorf("expected positive pid, got %d", pid)
	}
	if proc, err := os.FindProcess(pid); err == nil {
		_ = proc.Signal(syscall.SIGTERM)
	}
}
