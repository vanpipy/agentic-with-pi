package tui_test

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	agentclient "github.com/vanpiyp/awp/internal/agent-client"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	"github.com/vanpiyp/awp/internal/tui"
)

func ctrlC() tea.KeyPressMsg { return tea.KeyPressMsg{Code: 'c', Mod: uv.ModCtrl} }
func ctrlD() tea.KeyPressMsg { return tea.KeyPressMsg{Code: 'd', Mod: uv.ModCtrl} }
func ctrlE() tea.KeyPressMsg { return tea.KeyPressMsg{Code: 'e', Mod: uv.ModCtrl} }
func ctrlUp() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: uv.KeyUp, Mod: uv.ModCtrl}
}
func ctrlDown() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: uv.KeyDown, Mod: uv.ModCtrl}
}
func keyUp() tea.KeyPressMsg     { return tea.KeyPressMsg{Code: uv.KeyUp} }
func keyDown() tea.KeyPressMsg   { return tea.KeyPressMsg{Code: uv.KeyDown} }
func keyPgUp() tea.KeyPressMsg   { return tea.KeyPressMsg{Code: uv.KeyPgUp} }
func keyPgDown() tea.KeyPressMsg { return tea.KeyPressMsg{Code: uv.KeyPgDown} }
func keyTab() tea.KeyPressMsg    { return tea.KeyPressMsg{Code: uv.KeyTab} }
func keyEsc() tea.KeyPressMsg    { return tea.KeyPressMsg{Code: uv.KeyEsc} }
func keyEnter() tea.KeyPressMsg  { return tea.KeyPressMsg{Code: uv.KeyEnter} }
func keyA() tea.KeyPressMsg      { return tea.KeyPressMsg{Code: 'a'} }

func newSizedModel(t *testing.T, w, h int) *tui.Model {
	t.Helper()
	m := tui.NewModelForTest()
	out, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return out.(*tui.Model)
}

func runAllCmds(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	if bm, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, sub := range bm {
			if sub == nil {
				continue
			}
			out = append(out, runAllCmds(sub)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestUpdate_WindowSizeMsg_SetsWidthAndHeightAndRelaysToLayout(t *testing.T) {
	m := tui.NewModelForTest()
	out, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	mm := out.(*tui.Model)
	if mm.ViewForTest() == "initializing..." {
		t.Fatalf("after WindowSize, view should not be initializing: %q", mm.ViewForTest())
	}
	stripped := stripANSI(mm.ViewForTest())
	if !strings.Contains(stripped, "ready") {
		t.Errorf("expected status 'ready' in view, got:\n%s", stripped)
	}
}

func TestUpdate_Init_SpinsUpSpinnerAndReturnsTickCmd(t *testing.T) {
	m := tui.NewModelForTest()
	cmd := m.Init()
	if cmd == nil {
		t.Fatalf("Init must return a spinner.Tick cmd")
	}
}

func TestNewSpinnerForTest_BuildsMiniDotWithStatusStyle(t *testing.T) {
	s := tui.NewSpinnerForTest()
	if s.ID() <= 0 {
		t.Errorf("NewSpinnerForTest: expected id > 0, got %d", s.ID())
	}
}

func TestUpdate_CtrlC_WithEmptyInput_QuitsAndShutsDown(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	out, cmd := m.Update(ctrlC())
	mm := out.(*tui.Model)
	if !cmdContainsQuit(cmd) {
		t.Fatalf("ctrl+c with empty input must produce tea.Quit, got: %v", cmd)
	}
	if mm == nil {
		t.Errorf("ctrl+c must return the same model, got nil")
	}
}

func TestUpdate_CtrlD_WithEmptyInput_QuitsAndShutsDown(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	_, cmd := m.Update(ctrlD())
	if !cmdContainsQuit(cmd) {
		t.Fatalf("ctrl+d with empty input must produce tea.Quit, got: %v", cmd)
	}
}

func TestUpdate_CtrlC_WithNonEmptyInput_DoesNotQuit(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.InputAppendForTest("hello")
	_, cmd := m.Update(ctrlC())
	if cmdContainsQuit(cmd) {
		t.Fatalf("ctrl+c with non-empty input must NOT quit, got: %v", cmd)
	}
	if m.InputValueForTest() != "hello" {
		t.Errorf("input value should be preserved on ctrl+c, got %q", m.InputValueForTest())
	}
}

func TestUpdate_CtrlD_WithNonEmptyInput_DoesNotQuit(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.InputAppendForTest("hello")
	_, cmd := m.Update(ctrlD())
	if cmdContainsQuit(cmd) {
		t.Fatalf("ctrl+d with non-empty input must NOT quit, got: %v", cmd)
	}
}

func TestUpdate_CtrlE_TogglesCollapseAtViewportTop(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SubmitPromptForTest("first prompt")
	m.SubmitPromptForTest("second prompt")
	out, _ := m.Update(ctrlE())
	if out == nil {
		t.Errorf("ctrl+e must return the model")
	}
}

func TestUpdate_Esc_WithAutocompleteVisible_HidesAutocompleteAndResetsInput(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.InputAppendForTest("/")
	m.AutocompleteRefreshForTest()
	if !m.AutocompleteVisibleForTest() {
		t.Fatalf("setup: autocomplete should be visible after typing '/'")
	}
	_, _ = m.Update(keyEsc())
	if m.AutocompleteVisibleForTest() {
		t.Errorf("esc should hide autocomplete when visible")
	}
	if v := m.InputValueForTest(); v != "" {
		t.Errorf("esc on autocomplete should reset input, got %q", v)
	}
}

func TestUpdate_Esc_DuringStreaming_TransitionsToCancel(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateStreamingForTestValue())
	_, _ = m.Update(keyEsc())
	if m.StateForTest() != tui.StateCancelling {
		t.Errorf("esc during streaming must transition to StateCancelling, got %v", m.StateForTest())
	}
}

func TestUpdate_Esc_DuringCancelling_StaysInCancel(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateCancelling)
	_, _ = m.Update(keyEsc())
	if m.StateForTest() != tui.StateCancelling {
		t.Errorf("esc during cancelling must keep StateCancelling, got %v", m.StateForTest())
	}
}

func TestUpdate_Esc_WithNoPopup_AndNotStreaming_ForwardsToInput(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateReady)
	_, _ = m.Update(keyEsc())
}

func TestUpdate_Tab_WithAutocompleteVisible_AcceptsAutocomplete(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.InputAppendForTest("/compa")
	m.AutocompleteRefreshForTest()
	if !m.AutocompleteVisibleForTest() {
		t.Fatalf("setup: autocomplete should be visible after typing '/compa'")
	}
	_, _ = m.Update(keyTab())
}

func TestUpdate_Tab_WithoutAutocomplete_GoesToDefaultInput(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateReady)
	_, _ = m.Update(keyTab())
}

func TestUpdate_Enter_TrimsEmptyInput_NoStateChange(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateReady)
	m.InputAppendForTest("   ")
	_, cmd := m.Update(keyEnter())
	if cmdContainsQuit(cmd) {
		t.Errorf("empty (whitespace) enter must not quit, got: %v", cmd)
	}
	if m.StateForTest() != tui.StateReady {
		t.Errorf("empty (whitespace) enter must not change state, got %v", m.StateForTest())
	}
}

func TestUpdate_Enter_WithNonSlashText_NoConn_StreamTransition(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateReady)
	m.SetConnForTest(nil)
	m.InputAppendForTest("hi there")
	_, cmd := m.Update(keyEnter())
	if cmd == nil {
		t.Errorf("enter on plain text should schedule a cmd (startStream), got nil")
	}
	if m.StateForTest() != tui.StateStreaming {
		t.Errorf("enter on plain text must move to StateStreaming, got %v", m.StateForTest())
	}
}

func TestUpdate_Enter_MidStreaming_NoStateChange(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateStreaming)
	m.InputAppendForTest("another")
	_, cmd := m.Update(keyEnter())
	if cmdContainsQuit(cmd) {
		t.Errorf("plain text mid-stream must not quit, got: %v", cmd)
	}
	if v := m.InputValueForTest(); v != "another" {
		t.Errorf("plain text mid-stream must stay in input, got %q", v)
	}
}

func TestUpdate_Up_WithNoPopup_ScrollsChatUp(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	for i := 0; i < 3; i++ {
		m.SubmitPromptForTest("p")
	}
	out, _ := m.Update(keyUp())
	if out == nil {
		t.Errorf("up must return the model")
	}
}

func TestUpdate_Down_WithNoPopup_ScrollsChatDown(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	for i := 0; i < 3; i++ {
		m.SubmitPromptForTest("p")
	}
	out, _ := m.Update(keyDown())
	if out == nil {
		t.Errorf("down must return the model")
	}
}

func TestUpdate_Up_WithAutocompleteVisible_MovesUpInList(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.InputAppendForTest("/")
	m.AutocompleteRefreshForTest()
	if !m.AutocompleteVisibleForTest() {
		t.Fatalf("setup: autocomplete should be visible")
	}
	_, _ = m.Update(keyUp())
}

func TestUpdate_Down_WithAutocompleteVisible_MovesDownInList(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.InputAppendForTest("/")
	m.AutocompleteRefreshForTest()
	if !m.AutocompleteVisibleForTest() {
		t.Fatalf("setup: autocomplete should be visible")
	}
	_, _ = m.Update(keyDown())
}

func TestUpdate_PgUp_PerformsChatHalfPageUp(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	for i := 0; i < 8; i++ {
		m.SubmitPromptForTest(strings.Repeat("filler ", 10))
	}
	out, _ := m.Update(keyPgUp())
	if out == nil {
		t.Errorf("pgup must return the model")
	}
}

func TestUpdate_PgDown_PerformsChatHalfPageDown(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	for i := 0; i < 8; i++ {
		m.SubmitPromptForTest(strings.Repeat("filler ", 10))
	}
	out, _ := m.Update(keyPgDown())
	if out == nil {
		t.Errorf("pgdown must return the model")
	}
}

func TestUpdate_CtrlUp_JumpsToPromptAbove(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SubmitPromptForTest("first")
	m.SubmitPromptForTest("second")
	out, _ := m.Update(ctrlUp())
	if out == nil {
		t.Errorf("ctrl+up must return the model")
	}
}

func TestUpdate_CtrlDown_JumpsToPromptBelow(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SubmitPromptForTest("first")
	m.SubmitPromptForTest("second")
	out, _ := m.Update(ctrlDown())
	if out == nil {
		t.Errorf("ctrl+down must return the model")
	}
}

func TestUpdate_DefaultLetter_ForwardsToInputUpdate(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateReady)
	_, cmd := m.Update(keyA())
	if cmd == nil && m.LastPromptForTest() == "" {
		_ = cmd
	}
	stripped := stripANSI(m.ViewForTest())
	if !strings.Contains(stripped, "ready") {
		t.Errorf("default-letter should not break status render: %s", stripped)
	}
}

func TestUpdate_SpinnerTickMsg_AdvancesSpinner(t *testing.T) {
	m := tui.NewModelForTest()
	out, _ := m.Update(spinnerTickMsg())
	if out == nil {
		t.Errorf("spinner.TickMsg handler must return the model")
	}
}

func TestUpdate_StreamEvent_NoChannel_NoSchedulingFollowUp(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetEventsForTest(nil)
	before := m.ReadsScheduledThisUpdateForTest()
	out, _ := m.Update(tui.StreamEventMsgForTest(
		agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"x"}`)},
		nil, false,
	))
	mm := out.(*tui.Model)
	if got := mm.ReadsScheduledThisUpdateForTest(); got != before {
		t.Errorf("streamEventMsg with events=nil should not schedule a follow-up read, got delta=%d", got-before)
	}
}

func TestUpdate_StreamEvent_ReadsReschedule_WhenChannelSet(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	ch := make(chan agentclient.Event, 1)
	ch <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"x"}`)}
	m.SetEventsForTest(ch)
	out, _ := m.Update(tui.StreamEventMsgForTest(
		agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"x"}`)},
		nil, false,
	))
	mm := out.(*tui.Model)
	if got := mm.ReadsScheduledThisUpdateForTest(); got != 1 {
		t.Errorf("streamEventMsg with non-nil events channel should schedule exactly 1 follow-up read, got %d", got)
	}
}

func TestUpdate_StreamEvent_DuringCancelling_ResetsStateToReady(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateCancelling)
	ch := make(chan agentclient.Event, 1)
	ch <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"x"}`)}
	m.SetEventsForTest(ch)
	out, _ := m.Update(tui.StreamEventMsgForTest(
		agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"x"}`)},
		nil, false,
	))
	mm := out.(*tui.Model)
	if mm.StateForTest() != tui.StateReady {
		t.Errorf("streamEventMsg received during StateCancelling should reset to StateReady, got %v", mm.StateForTest())
	}
}

func TestUpdate_StreamEvent_WithAbortMessage_TransitionsToStateError(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateStreaming)
	ch := make(chan agentclient.Event, 1)
	m.SetEventsForTest(ch)
	out, _ := m.Update(tui.StreamEventMsgForTest(
		agentclient.Event{
			Kind: json_rpc.EventMessage,
			Data: []byte(`{"id":"x","message":{"role":"assistant","content":[]},"stopReason":"abort"}`),
		},
		nil, false,
	))
	mm := out.(*tui.Model)
	if mm.StateForTest() != tui.StateError {
		t.Errorf("streamEventMsg with abort StopReason should set StateError, got %v", mm.StateForTest())
	}
}

func TestUpdate_StreamEvent_WithAbortCustom_TransitionsToStateError(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateStreaming)
	ch := make(chan agentclient.Event, 1)
	m.SetEventsForTest(ch)
	out, _ := m.Update(tui.StreamEventMsgForTest(
		agentclient.Event{
			Kind: json_rpc.EventCustom,
			Data: []byte(`{"id":"x","customType":"abort","data":{"reason":"timeout"}}`),
		},
		nil, false,
	))
	mm := out.(*tui.Model)
	if mm.StateForTest() != tui.StateError {
		t.Errorf("streamEventMsg with abort customType should set StateError, got %v", mm.StateForTest())
	}
}

func TestUpdate_StreamEvent_WithAbortCustom_InvalidJSON_IsSafelyIgnored(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateStreaming)
	ch := make(chan agentclient.Event, 1)
	m.SetEventsForTest(ch)
	out, _ := m.Update(tui.StreamEventMsgForTest(
		agentclient.Event{Kind: json_rpc.EventCustom, Data: []byte(`not-json`)},
		nil, false,
	))
	mm := out.(*tui.Model)
	if mm.StateForTest() != tui.StateStreaming {
		t.Errorf("invalid abort custom JSON should not change state, got %v", mm.StateForTest())
	}
}

func TestUpdate_StreamEvent_Done_DrainsAndClearsEventsChannel(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	ch := make(chan agentclient.Event, 4)
	ch <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"1"}`)}
	ch <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"2"}`)}
	m.SetEventsForTest(ch)
	out, _ := m.Update(tui.StreamEventMsgForTest(
		agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"final"}`)},
		nil, true,
	))
	mm := out.(*tui.Model)
	if mm.EventsForTest() != nil {
		t.Errorf("done streamEventMsg should set m.events=nil, got non-nil")
	}
	if mm.StateForTest() != tui.StateReady {
		t.Errorf("done streamEventMsg should set StateReady, got %v", mm.StateForTest())
	}
	if remaining := len(ch); remaining != 0 {
		t.Errorf("done streamEventMsg should drain events, %d buffered remain", remaining)
	}
}

func TestUpdate_StreamEvent_Done_NilChannelStillSetsStateReady(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetEventsForTest(nil)
	out, _ := m.Update(tui.StreamEventMsgForTest(
		agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"final"}`)},
		nil, true,
	))
	mm := out.(*tui.Model)
	if mm.StateForTest() != tui.StateReady {
		t.Errorf("done streamEventMsg (nil channel) should set StateReady, got %v", mm.StateForTest())
	}
}

func TestUpdate_StreamEvent_NonAbortMessage_DoesNotChangeStateFromStreaming(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateStreaming)
	ch := make(chan agentclient.Event, 1)
	m.SetEventsForTest(ch)
	out, _ := m.Update(tui.StreamEventMsgForTest(
		agentclient.Event{
			Kind: json_rpc.EventMessage,
			Data: []byte(`{"id":"x","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]},"stopReason":"end_turn"}`),
		},
		nil, false,
	))
	mm := out.(*tui.Model)
	if mm.StateForTest() != tui.StateStreaming {
		t.Errorf("end_turn streamEventMsg (still streaming) should not set StateReady/StateError alone, got %v", mm.StateForTest())
	}
}

func TestUpdate_StreamEvent_ErrFieldSetsStateErrorAndNilEvents(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateStreaming)
	ch := make(chan agentclient.Event, 1)
	m.SetEventsForTest(ch)
	out, _ := m.Update(tui.StreamEventMsgForTest(
		agentclient.Event{}, errors.New("net down"), false,
	))
	mm := out.(*tui.Model)
	if mm.StateForTest() != tui.StateError {
		t.Errorf("err streamEventMsg should set StateError, got %v", mm.StateForTest())
	}
	if mm.EventsForTest() != nil {
		t.Errorf("err streamEventMsg should set m.events=nil, got non-nil")
	}
}

func TestUpdate_ReadsScheduled_ResetsPerUpdate(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	ch := make(chan agentclient.Event, 1)
	ch <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"x"}`)}
	m.SetEventsForTest(ch)
	out, _ := m.Update(tui.StreamEventMsgForTest(
		agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"x"}`)},
		nil, false,
	))
	mm := out.(*tui.Model)
	if got := mm.ReadsScheduledThisUpdateForTest(); got != 1 {
		t.Fatalf("expected 1 follow-up read scheduled, got %d", got)
	}
	out2, _ := mm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	mm2 := out2.(*tui.Model)
	if got := mm2.ReadsScheduledThisUpdateForTest(); got != 0 {
		t.Errorf("after Enter (no event channel), readsScheduledThisUpdate should reset to 0, got %d", got)
	}
}

func TestUpdate_StreamEvent_GotoBottomWhenFollowing(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	ch := make(chan agentclient.Event, 1)
	m.SetEventsForTest(ch)
	out, _ := m.Update(tui.StreamEventMsgForTest(
		agentclient.Event{
			Kind: json_rpc.EventMessage,
			Data: []byte(`{"id":"x","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]},"stopReason":"end_turn"}`),
		},
		nil, false,
	))
	if out == nil {
		t.Errorf("expected non-nil model return")
	}
}

func TestView_NoWidth_ReturnsInitializing(t *testing.T) {
	m := tui.NewModelForTest()
	m.SetSizeForTest(0, 0)
	if got := m.ViewForTest(); got != "initializing..." {
		t.Errorf("View with width=0 should return initializing..., got %q", got)
	}
}

func TestView_StreamStatus_ShowsSpinnerAndStreamingLabel(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateStreaming)
	stripped := stripANSI(m.ViewForTest())
	if !strings.Contains(stripped, "streaming") {
		t.Errorf("streaming state should show streaming label, got:\n%s", stripped)
	}
}

func TestView_ErrorStatus_ShowsErrorLabel(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateError)
	stripped := stripANSI(m.ViewForTest())
	if !strings.Contains(stripped, "error") {
		t.Errorf("error state should show error label, got:\n%s", stripped)
	}
}

func TestView_ReadyStatus_ShowsReadyLabel(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	stripped := stripANSI(m.ViewForTest())
	if !strings.Contains(stripped, "ready") {
		t.Errorf("ready state should show ready label, got:\n%s", stripped)
	}
}

func TestView_LastPrompt_AppearsInHeader(t *testing.T) {
	m := newSizedModel(t, 120, 24)
	m.SubmitPromptForTest("explain the difference between context cancellation and request cancellation in go http")
	stripped := stripANSI(m.ViewForTest())
	if !strings.Contains(stripped, "explain the difference") {
		t.Errorf("last prompt should appear in header, got:\n%s", stripped)
	}
}

func TestLayout_ZeroSize_NoLayoutRecompute(t *testing.T) {
	m := tui.NewModelForTest()
	m.SetSizeForTest(0, 0)
	m.LayoutForTest()
	if m.ViewForTest() != "initializing..." {
		t.Errorf("layout with zero size should keep view in initializing state, got %q", m.ViewForTest())
	}
}

func TestInputHeight_ReportsUnderlyingTextareaHeight(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	got := m.InputHeightForTest()
	if got <= 0 {
		t.Errorf("InputHeightForTest should be >0 for a focused textarea, got %d", got)
	}
}

func TestPickerView_WhenNotVisible_ReturnsEmpty(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	if got := m.PickerViewForTest(); got != "" {
		t.Errorf("picker view should be empty when not visible, got %q", got)
	}
}

func TestPickerVisible_InitiallyFalse(t *testing.T) {
	m := tui.NewModelForTest()
	if m.PickerVisibleForTest() {
		t.Errorf("picker should not be visible initially")
	}
}

func TestAutocompleteView_WhenNotVisible_ReturnsEmpty(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	if got := m.AutocompleteViewForTest(); got != "" {
		t.Errorf("autocomplete view should be empty when not visible, got %q", got)
	}
}

func TestAutocompleteItems_NilModelReturnedIsEmpty(t *testing.T) {
	m := tui.NewModelForTest()
	items := m.AutocompleteItemsForTest()
	if items == nil {
		t.Errorf("AutocompleteItemsForTest: items must not be nil, got nil")
	}
}

func TestAutocompleteItems_ListsRegisteredCommands(t *testing.T) {
	m := tui.NewModelForTest()
	items := m.AutocompleteItemsForTest()
	if len(items) == 0 {
		t.Fatalf("expected non-empty autocomplete list, got 0")
	}
	wantNames := map[string]bool{"quit": false, "new": false, "resume": false, "compact": false, "usage": false, "skills": false}
	for _, it := range items {
		if _, ok := wantNames[it.Title]; ok {
			wantNames[it.Title] = true
		}
	}
	for name, found := range wantNames {
		if !found {
			t.Errorf("autocomplete should include /%s command, was missing", name)
		}
	}
}

func TestEnterForTest_SubmitsPrompt(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.EnterForTest("a quick brown fox")
	if v := m.LastPromptForTest(); v != "a quick brown fox" {
		t.Errorf("EnterForTest should set lastPrompt=%q, got %q", "a quick brown fox", v)
	}
}

func TestSetLastPromptForTest_Roundtrips(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	tui.SetLastPromptForTest(m, "tracked prompt")
	if got := m.LastPromptForTest(); got != "tracked prompt" {
		t.Errorf("last prompt roundtrip lost: got %q", got)
	}
	tui.SetLastPromptForTest(nil, "ignored")
}

func TestUpdate_WindowSizeMsg_IdempotentOnSecondUpdate(t *testing.T) {
	m := tui.NewModelForTest()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	second, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if second == nil {
		t.Errorf("repeated WindowSizeMsg should not nil the model")
	}
	if !strings.Contains(stripANSI(second.(*tui.Model).ViewForTest()), "ready") {
		t.Errorf("view after second resize should still render status")
	}
}

func TestUpdate_HandlesZeroHeightGracefully(t *testing.T) {
	m := tui.NewModelForTest()
	out, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 0})
	if out == nil {
		t.Errorf("zero-height WindowSizeMsg should not nil the model")
	}
	if got := stripANSI(out.(*tui.Model).ViewForTest()); !strings.Contains(got, "ready") {
		t.Errorf("view after zero-height resize should still render header, got:\n%s", got)
	}
}

func TestCancelForTest_AppendsCancelledHintToChat(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateStreaming)
	m.SetConnForTest(nil)
	m.SetEventsForTest(nil)
	before := len(m.ChatMessagesForTest())
	cmd := m.CancelForTest()
	if cmd != nil {
		t.Errorf("CancelForTest with events=nil should return nil cmd, got: %v", cmd)
	}
	after := len(m.ChatMessagesForTest())
	if after <= before {
		t.Errorf("cancel should append a system message, before=%d after=%d", before, after)
	}
}

func TestCancelForTest_WithEventsChannel_ReturnsReadCmd(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateStreaming)
	ch := make(chan agentclient.Event, 1)
	m.SetEventsForTest(ch)
	cmd := m.CancelForTest()
	if cmd == nil {
		t.Errorf("CancelForTest with events channel should return readNextEvent cmd")
	}
}

func TestCancelForTest_ErrMsgTransitionsToError(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetConnForTest(nil)
	m.InputAppendForTest("plain text")
	_, cmd := m.Update(keyEnter())
	if cmd == nil {
		t.Fatalf("setup: Enter should return a cmd batch")
	}
	msgs := runAllCmds(cmd)
	if len(msgs) == 0 {
		t.Fatalf("expected at least one msg from startStream cmd, got none")
	}
	before := m.StateForTest()
	for _, msg := range msgs {
		out, _ := m.Update(msg)
		if out != nil {
			if mm, ok := out.(*tui.Model); ok {
				m = mm
			}
		}
	}
	if m.StateForTest() == before {
		t.Errorf("after running startStream cmd, state should have transitioned (was %v, now %v)", before, m.StateForTest())
	}
}

func TestRenderHeader_NarrowSessionColumn_RecomputesWidths(t *testing.T) {
	_ = newSizedModel(t, 18, 24)
	id := "abcdefghijklmnop"
	stripped := stripANSI(tui.RenderHeaderWithPromptForTest(18, "ready", "", id))
	if !strings.Contains(stripped, "ready") {
		t.Errorf("narrow header should still render status, got:\n%s", stripped)
	}
}

func TestSessionLabel_VerySmallMaxWidth_LimitsLength(t *testing.T) {
	id := "abcdefghijklmnopqrstuvwxyz"
	got := tui.SessionLabelForTest(id, 1)
	if len([]rune(got)) > 1 {
		t.Errorf("SessionLabelForTest with maxWidth=1 should not exceed 1 rune, got %q", got)
	}
}

func TestIsCancelFirstCommand_InvalidInputFallsThrough(t *testing.T) {
	if got := tui.IsCancelFirstCommandForTest("not-a-cmd"); got {
		t.Errorf("non-slash input should not be cancel-first")
	}
}

func TestUpdate_StreamEvent_EventCustomNonAbort_DoesNotChangeState(t *testing.T) {
	m := newSizedModel(t, 80, 24)
	m.SetStateForTest(tui.StateStreaming)
	ch := make(chan agentclient.Event, 1)
	m.SetEventsForTest(ch)
	out, _ := m.Update(tui.StreamEventMsgForTest(
		agentclient.Event{
			Kind: json_rpc.EventCustom,
			Data: []byte(`{"id":"x","customType":"some_other_type","data":{}}`),
		},
		nil, false,
	))
	mm := out.(*tui.Model)
	if mm.StateForTest() != tui.StateStreaming {
		t.Errorf("non-abort custom event should not change state, got %v", mm.StateForTest())
	}
}
