package agentcore_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	"github.com/vanpiyp/awp/internal/tui"
)

func TestChatMsgExposesTitleField(t *testing.T) {
	got := tui.ChatMsgFromTest(tui.ChatMsg{
		Role:  tui.RoleTool,
		Title: "list /tmp",
	})
	if got.Title != "list /tmp" {
		t.Errorf("ChatMsg.Title = %q, want %q", got.Title, "list /tmp")
	}
}

func TestChatMsgExposesToolCallsField(t *testing.T) {
	got := tui.ChatMsgFromTest(tui.ChatMsg{
		Role:      tui.RoleAssistant,
		ToolCalls: []string{"bash", "read"},
	})
	if len(got.ToolCalls) != 2 {
		t.Fatalf("ChatMsg.ToolCalls len = %d, want 2", len(got.ToolCalls))
	}
	if got.ToolCalls[0] != "bash" || got.ToolCalls[1] != "read" {
		t.Errorf("ChatMsg.ToolCalls = %v, want [bash read]", got.ToolCalls)
	}
}

func TestChatMsgExposesToolDataField(t *testing.T) {
	part := &json_rpc.MessageContentPart{
		Type:      "toolCall",
		ID:        "call-1",
		Name:      "bash",
		Intent:    "list files",
		Arguments: json.RawMessage(`{"path":"/tmp"}`),
	}
	got := tui.ChatMsgFromTest(tui.ChatMsg{
		Role:     tui.RoleTool,
		ToolData: part,
	})
	if got.ToolData == nil {
		t.Fatal("ChatMsg.ToolData = nil, want non-nil")
	}
	if got.ToolData.Name != "bash" {
		t.Errorf("ChatMsg.ToolData.Name = %q, want %q", got.ToolData.Name, "bash")
	}
	if got.ToolData.Intent != "list files" {
		t.Errorf("ChatMsg.ToolData.Intent = %q, want %q", got.ToolData.Intent, "list files")
	}
	if string(got.ToolData.Arguments) != `{"path":"/tmp"}` {
		t.Errorf("ChatMsg.ToolData.Arguments = %s, want %s", string(got.ToolData.Arguments), `{"path":"/tmp"}`)
	}
}

func TestChatMsgBackwardCompatDefaultsEmpty(t *testing.T) {
	got := tui.ChatMsgFromTest(tui.ChatMsg{
		Role: tui.RoleUser,
		Text: "hello",
	})
	if got.Title != "" {
		t.Errorf("default Title = %q, want empty", got.Title)
	}
	if got.ToolCalls != nil {
		t.Errorf("default ToolCalls = %v, want nil", got.ToolCalls)
	}
	if got.ToolData != nil {
		t.Errorf("default ToolData = %v, want nil", got.ToolData)
	}
}

func TestAppendToolPopulatesToolData(t *testing.T) {
	m := tui.NewChatModelForTest()
	part := json_rpc.MessageContentPart{
		Type:      "toolCall",
		ID:        "call-42",
		Name:      "bash",
		Intent:    "list /tmp",
		Arguments: json.RawMessage(`{"path":"/tmp"}`),
	}
	tui.AppendToolForTest(m, part)
	last := tui.LastChatMsgForTest(m)
	if last.Role != tui.RoleTool {
		t.Fatalf("last Role = %v, want RoleTool", last.Role)
	}
	if last.ToolData == nil {
		t.Fatal("last ToolData = nil after AppendToolForTest, want populated")
	}
	if last.ToolData.ID != "call-42" {
		t.Errorf("ToolData.ID = %q, want %q", last.ToolData.ID, "call-42")
	}
	if last.ToolData.Name != "bash" {
		t.Errorf("ToolData.Name = %q, want %q", last.ToolData.Name, "bash")
	}
	if last.ToolData.Intent != "list /tmp" {
		t.Errorf("ToolData.Intent = %q, want %q", last.ToolData.Intent, "list /tmp")
	}
	if string(last.ToolData.Arguments) != `{"path":"/tmp"}` {
		t.Errorf("ToolData.Arguments = %s, want %s", string(last.ToolData.Arguments), `{"path":"/tmp"}`)
	}
}

func TestAppendToolTextMatchesLegacyFormat(t *testing.T) {
	m := tui.NewChatModelForTest()
	part := json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "bash",
		Intent:    "list /tmp",
		Arguments: json.RawMessage(`{"path":"/tmp"}`),
	}
	tui.AppendToolForTest(m, part)
	last := tui.LastChatMsgForTest(m)
	want := "list /tmp · bash({\"path\":\"/tmp\"})"
	if last.Text != want {
		t.Errorf("AppendTool text = %q, want %q (zero render regression)", last.Text, want)
	}
}

func TestAppendToolTextMatchesWithoutIntent(t *testing.T) {
	m := tui.NewChatModelForTest()
	part := json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "bash",
		Arguments: json.RawMessage(`{"path":"/tmp"}`),
	}
	tui.AppendToolForTest(m, part)
	last := tui.LastChatMsgForTest(m)
	want := "bash({\"path\":\"/tmp\"})"
	if last.Text != want {
		t.Errorf("AppendTool text (no intent) = %q, want %q", last.Text, want)
	}
}

func TestAppendToolRenderUsesCardPathWhenToolDataPopulated(t *testing.T) {
	legacy := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleTool,
		Text: "list /tmp · bash({\"path\":\"/tmp\"})",
	}, 80)
	if len(legacy) == 0 {
		t.Fatal("legacy render produced zero lines; cannot compare")
	}

	m := tui.NewChatModelForTest()
	part := json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "bash",
		Intent:    "list /tmp",
		Arguments: json.RawMessage(`{"path":"/tmp"}`),
	}
	tui.AppendToolForTest(m, part)
	last := tui.LastChatMsgForTest(m)

	card := tui.RenderMsgForTest(last, 80)
	if len(card) <= len(legacy) {
		t.Fatalf("card render must produce more lines than legacy: legacy=%d card=%d",
			len(legacy), len(card))
	}
}

func TestAppendToolRenderIgnoresTitleAndToolCallsWhenToolDataNil(t *testing.T) {
	baseline := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleTool,
		Text: "list /tmp · bash({\"path\":\"/tmp\"})",
	}, 80)
	enriched := tui.RenderMsgForTest(tui.ChatMsg{
		Role:      tui.RoleTool,
		Text:      "list /tmp · bash({\"path\":\"/tmp\"})",
		Title:     "ignored title",
		ToolCalls: []string{"bash", "read"},
	}, 80)
	if len(baseline) != len(enriched) {
		t.Fatalf("render line count drift with Title/ToolCalls populated: baseline=%d enriched=%d",
			len(baseline), len(enriched))
	}
	for i := range baseline {
		if baseline[i] != enriched[i] {
			t.Errorf("render line %d drift with Title/ToolCalls populated:\n baseline=%q\n enriched=%q",
				i, baseline[i], enriched[i])
		}
	}
}

func TestRenderMsgBackwardCompatWithoutNewFields(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleUser,
		Text: "hello world",
	}, 80)
	if len(lines) == 0 {
		t.Fatal("backward-compat render produced zero lines")
	}
	found := false
	for _, l := range lines {
		if strings.Contains(l, "hello world") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("backward-compat render missing text, lines=%v", lines)
	}
}
