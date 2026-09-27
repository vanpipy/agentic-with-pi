package tui_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	"github.com/vanpiyp/awp/internal/tui"
)

func TestSummarizeToolArgReadWithStartAndEnd(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "read",
		Arguments: json.RawMessage(`{"file":"x.go","start_line":10,"end_line":20}`),
	})
	joined := strings.Join(tui.RenderLastMessageForTest(c, 120), "\n")
	if !strings.Contains(joined, "x.go:10-20") {
		t.Errorf("read sOK+eOK should render file:start-end, got %q", joined)
	}
}

func TestSummarizeToolArgReadWithStartOnly(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "read",
		Arguments: json.RawMessage(`{"file":"x.go","start_line":42}`),
	})
	joined := strings.Join(tui.RenderLastMessageForTest(c, 120), "\n")
	if !strings.Contains(joined, "x.go:42-") {
		t.Errorf("read sOK only should render file:start-, got %q", joined)
	}
}

func TestSummarizeToolArgReadWithEndOnly(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "read",
		Arguments: json.RawMessage(`{"file":"y.go","end_line":50}`),
	})
	joined := strings.Join(tui.RenderLastMessageForTest(c, 120), "\n")
	if !strings.Contains(joined, "y.go:1-50") {
		t.Errorf("read eOK only should render file:1-end, got %q", joined)
	}
}

func TestSummarizeToolArgReadWithOffsetAndLimit(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "read",
		Arguments: json.RawMessage(`{"file":"z.go","offset":100,"limit":50}`),
	})
	joined := strings.Join(tui.RenderLastMessageForTest(c, 120), "\n")
	if !strings.Contains(joined, "z.go:100-150") {
		t.Errorf("read oOK+lOK should render file:offset-(offset+limit), got %q", joined)
	}
}

func TestSummarizeToolArgReadWithOffsetOnly(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "read",
		Arguments: json.RawMessage(`{"file":"w.go","offset":200}`),
	})
	joined := strings.Join(tui.RenderLastMessageForTest(c, 120), "\n")
	if !strings.Contains(joined, "w.go:200") {
		t.Errorf("read oOK only should render file:offset (no dash), got %q", joined)
	}
}

func TestSummarizeToolArgReadFileOnlyNoSuffix(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "read",
		Arguments: json.RawMessage(`{"file":"just-file.go"}`),
	})
	joined := strings.Join(tui.RenderLastMessageForTest(c, 120), "\n")
	if !strings.Contains(joined, "just-file.go") {
		t.Errorf("read file-only should render bare file, got %q", joined)
	}
	if strings.Contains(joined, "just-file.go:") {
		t.Errorf("read file-only should not include ':' suffix, got %q", joined)
	}
}

func TestSummarizeToolArgWriteEditEditMatch(t *testing.T) {
	for _, name := range []string{"write", "edit", "edit_match"} {
		t.Run(name, func(t *testing.T) {
			c := tui.NewChatModelForTest()
			tui.AppendToolForTest(c, json_rpc.MessageContentPart{
				Type:      "toolCall",
				Name:      name,
				Arguments: json.RawMessage(`{"file":"target.go","new_string":"x"}`),
			})
			joined := strings.Join(tui.RenderLastMessageForTest(c, 120), "\n")
			if !strings.Contains(joined, "target.go") {
				t.Errorf("%s: should render file only, got %q", name, joined)
			}
		})
	}
}

func TestSummarizeToolArgBashRendersCommand(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "bash",
		Arguments: json.RawMessage(`{"command":"ls -la"}`),
	})
	joined := strings.Join(tui.RenderLastMessageForTest(c, 120), "\n")
	if !strings.Contains(joined, "$ ls -la") {
		t.Errorf("bash should render $ command, got %q", joined)
	}
}

func TestSummarizeToolArgGrepWithPath(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "grep",
		Arguments: json.RawMessage(`{"query":"foo","path":"/repo/src"}`),
	})
	joined := strings.Join(tui.RenderLastMessageForTest(c, 120), "\n")
	if !strings.Contains(joined, "/foo/") {
		t.Errorf("grep with path should render /query/, got %q", joined)
	}
	if !strings.Contains(joined, "/repo/src") {
		t.Errorf("grep with path should include path, got %q", joined)
	}
}

func TestSummarizeToolArgGrepWithoutPath(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "grep",
		Arguments: json.RawMessage(`{"query":"foo"}`),
	})
	joined := strings.Join(tui.RenderLastMessageForTest(c, 120), "\n")
	if !strings.Contains(joined, "/foo/") {
		t.Errorf("grep no-path should render /query/, got %q", joined)
	}
}

func TestSummarizeToolArgAgentgrepBranches(t *testing.T) {
	t.Run("with_path", func(t *testing.T) {
		c := tui.NewChatModelForTest()
		tui.AppendToolForTest(c, json_rpc.MessageContentPart{
			Type:      "toolCall",
			Name:      "agentgrep",
			Arguments: json.RawMessage(`{"query":"bar","path":"/x"}`),
		})
		joined := strings.Join(tui.RenderLastMessageForTest(c, 120), "\n")
		if !strings.Contains(joined, "/bar/") || !strings.Contains(joined, "/x") {
			t.Errorf("agentgrep with path should render /bar/ + path, got %q", joined)
		}
	})
	t.Run("no_path", func(t *testing.T) {
		c := tui.NewChatModelForTest()
		tui.AppendToolForTest(c, json_rpc.MessageContentPart{
			Type:      "toolCall",
			Name:      "agentgrep",
			Arguments: json.RawMessage(`{"query":"baz"}`),
		})
		joined := strings.Join(tui.RenderLastMessageForTest(c, 120), "\n")
		if !strings.Contains(joined, "/baz/") {
			t.Errorf("agentgrep no-path should render /baz/, got %q", joined)
		}
	})
}

func TestSummarizeToolArgFindBranches(t *testing.T) {
	t.Run("with_path", func(t *testing.T) {
		c := tui.NewChatModelForTest()
		tui.AppendToolForTest(c, json_rpc.MessageContentPart{
			Type:      "toolCall",
			Name:      "find",
			Arguments: json.RawMessage(`{"query":"thing","path":"/p"}`),
		})
		joined := strings.Join(tui.RenderLastMessageForTest(c, 120), "\n")
		if !strings.Contains(joined, "thing") || !strings.Contains(joined, "/p") {
			t.Errorf("find with path should render query + path, got %q", joined)
		}
	})
	t.Run("no_path", func(t *testing.T) {
		c := tui.NewChatModelForTest()
		tui.AppendToolForTest(c, json_rpc.MessageContentPart{
			Type:      "toolCall",
			Name:      "find",
			Arguments: json.RawMessage(`{"query":"alone"}`),
		})
		joined := strings.Join(tui.RenderLastMessageForTest(c, 120), "\n")
		if !strings.Contains(joined, "alone") {
			t.Errorf("find no-path should render query, got %q", joined)
		}
	})
}

func TestSummarizeToolArgGlob(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "glob",
		Arguments: json.RawMessage(`{"pattern":"**/*.go"}`),
	})
	joined := strings.Join(tui.RenderLastMessageForTest(c, 120), "\n")
	if !strings.Contains(joined, "**/*.go") {
		t.Errorf("glob should render pattern, got %q", joined)
	}
}

func TestSummarizeToolArgLsBranches(t *testing.T) {
	t.Run("with_path", func(t *testing.T) {
		c := tui.NewChatModelForTest()
		tui.AppendToolForTest(c, json_rpc.MessageContentPart{
			Type:      "toolCall",
			Name:      "ls",
			Arguments: json.RawMessage(`{"path":"/some/dir"}`),
		})
		joined := strings.Join(tui.RenderLastMessageForTest(c, 120), "\n")
		if !strings.Contains(joined, "/some/dir") {
			t.Errorf("ls with path should render path, got %q", joined)
		}
	})
	t.Run("no_path", func(t *testing.T) {
		c := tui.NewChatModelForTest()
		tui.AppendToolForTest(c, json_rpc.MessageContentPart{
			Type:      "toolCall",
			Name:      "ls",
			Arguments: json.RawMessage(`{}`),
		})
		joined := strings.Join(tui.RenderLastMessageForTest(c, 120), "\n")
		if !strings.Contains(joined, ".") {
			t.Errorf("ls no-path should render '.', got %q", joined)
		}
	})
}

func TestSummarizeToolArgUnknownToolFallsBackToTruncate(t *testing.T) {
	c := tui.NewChatModelForTest()
	args := json.RawMessage(`{"arbitrary":"value","another":123}`)
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "no_such_tool",
		Arguments: args,
	})
	joined := strings.Join(tui.RenderLastMessageForTest(c, 120), "\n")
	if !strings.Contains(joined, "arbitrary") {
		t.Errorf("unknown tool should render args JSON via TruncateMiddle, got %q", joined)
	}
}

func TestSummarizeToolArgEmptyArgsSkipsSummary(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "read",
		Arguments: nil,
	})
	lines := tui.RenderLastMessageForTest(c, 120)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "read") {
		t.Errorf("tool name should still appear, got %q", joined)
	}
}

func TestSummarizeToolArgNullArgsSkipsSummary(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "read",
		Arguments: json.RawMessage(`null`),
	})
	lines := tui.RenderLastMessageForTest(c, 120)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "read") {
		t.Errorf("tool name should still appear, got %q", joined)
	}
}

func TestSummarizeToolArgInvalidJSONFallsBackToTruncate(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "bash",
		Arguments: json.RawMessage(`not-valid-json`),
	})
	joined := strings.Join(tui.RenderLastMessageForTest(c, 120), "\n")
	if !strings.Contains(joined, "not-valid-json") {
		t.Errorf("invalid JSON should fall back to TruncateMiddle of raw args, got %q", joined)
	}
}

func TestFormatTokenBadgeThousandsBoundary(t *testing.T) {
	cases := []struct {
		size int
		want string
	}{
		{500, "125 tok"},
		{999 * 4, "999 tok"},
		{1000 * 4, "1k tok"},
		{1200 * 4, "1.2k tok"},
		{2500 * 4, "2.5k tok"},
		{10000 * 4, "10k tok"},
		{100000 * 4, "100k tok"},
	}
	for _, tc := range cases {
		c := tui.NewChatModelForTest()
		tui.AppendToolForTest(c, json_rpc.MessageContentPart{
			Type:      "toolCall",
			Name:      "read",
			Arguments: json.RawMessage(`{"file":"x.go"}`),
		})
		tui.AppendObserveForTest(c, strings.Repeat("a", tc.size), "")
		joined := strings.Join(tui.RenderLastMessageForTest(c, 120), "\n")
		if !strings.Contains(joined, tc.want) {
			t.Errorf("size=%d: expected badge %q in output, got %q", tc.size, tc.want, joined)
		}
	}
}

func TestFormatIntDrivesCommaInsertionThroughRenderToolRow(t *testing.T) {
	cases := []struct {
		startLine int
		endLine   int
		wantStart string
		wantEnd   string
	}{
		{1234, 5678, "1,234", "5,678"},
		{100000, 123456, "100,000", "123,456"},
	}
	for _, tc := range cases {
		c := tui.NewChatModelForTest()
		tui.AppendToolForTest(c, json_rpc.MessageContentPart{
			Type:      "toolCall",
			Name:      "read",
			Arguments: json.RawMessage(fmt.Sprintf(`{"file":"a.go","start_line":%d,"end_line":%d}`, tc.startLine, tc.endLine)),
		})
		joined := strings.Join(tui.RenderLastMessageForTest(c, 120), "\n")
		if !strings.Contains(joined, tc.wantStart) {
			t.Errorf("start_line=%d should render %s, got %q", tc.startLine, tc.wantStart, joined)
		}
		if !strings.Contains(joined, tc.wantEnd) {
			t.Errorf("end_line=%d should render %s, got %q", tc.endLine, tc.wantEnd, joined)
		}
	}
}
