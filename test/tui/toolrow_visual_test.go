package tui_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	"github.com/vanpiyp/awp/internal/tui"
)

func TestToolRow_Format_Success(t *testing.T) {
	c := tui.NewChatModelForTest()
	td := json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "read",
		Intent:    "Read config section",
		Arguments: []byte(`{"file":"/home/leroy/Project/agentic-with-pi/internal/tui/app.go","start_line":270}`),
	}
	tui.AppendToolForTest(c, td)
	tui.AppendObserveForTest(c, "file contents here\nline 2\nline 3\n", "")

	lines := tui.RenderLastMessageForTest(c, 120)
	for i, l := range lines {
		t.Logf("L%d: %s", i, l)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "read") {
		t.Errorf("expected row to mention tool name, got %q", joined)
	}
	if !strings.Contains(joined, "270") {
		t.Errorf("expected row to mention start_line, got %q", joined)
	}
	if !strings.Contains(joined, "tok") {
		t.Errorf("expected row to include token badge, got %q", joined)
	}
}

func TestToolRow_Format_Running(t *testing.T) {
	c := tui.NewChatModelForTest()
	td := json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "read",
		Intent:    "Read config section",
		Arguments: []byte(`{"file":"/home/leroy/Project/agentic-with-pi/internal/tui/app.go","start_line":270}`),
	}
	tui.AppendToolForTest(c, td)

	lines := tui.RenderLastMessageForTest(c, 120)
	for i, l := range lines {
		t.Logf("L%d: %s", i, l)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "read") {
		t.Errorf("expected running row to mention tool name, got %q", joined)
	}
	if strings.Contains(joined, "tok") {
		t.Errorf("running row should not include token badge, got %q", joined)
	}
}

func TestToolRow_Format_Failed(t *testing.T) {
	c := tui.NewChatModelForTest()
	td := json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "bash",
		Intent:    "Run failing command",
		Arguments: []byte(`{"command":"false"}`),
	}
	tui.AppendToolForTest(c, td)
	tui.AppendObserveForTest(c, "Error: exit code 1\n", "")

	lines := tui.RenderLastMessageForTest(c, 120)
	for i, l := range lines {
		t.Logf("L%d: %s", i, l)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "bash") {
		t.Errorf("expected failed row to mention tool name, got %q", joined)
	}
}

func TestToolRow_Format_Bash(t *testing.T) {
	c := tui.NewChatModelForTest()
	td := json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "bash",
		Intent:    "Check git status",
		Arguments: []byte(`{"command":"git status && echo done"}`),
	}
	tui.AppendToolForTest(c, td)
	tui.AppendObserveForTest(c, "On branch main\nnothing to commit, working tree clean\n", "")

	lines := tui.RenderLastMessageForTest(c, 120)
	for i, l := range lines {
		t.Logf("L%d: %s", i, l)
	}
}

func TestToolRow_Format_LargeResultWarningColor(t *testing.T) {
	c := tui.NewChatModelForTest()
	td := json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "read",
		Arguments: []byte(`{"file":"a.txt"}`),
	}
	tui.AppendToolForTest(c, td)
	tui.AppendObserveForTest(c, strings.Repeat("a", 20000), "")

	lines := tui.RenderLastMessageForTest(c, 120)
	for i, l := range lines {
		t.Logf("L%d: %s", i, l)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "tok") {
		t.Errorf("expected token badge, got %q", joined)
	}
	if !strings.Contains(joined, "k tok") {
		t.Errorf("expected 5k+ token badge for 20kB result, got %q", joined)
	}
}
