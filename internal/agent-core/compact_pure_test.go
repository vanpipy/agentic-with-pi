package agentcore

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
)

func TestPureHelpersExtracted(t *testing.T) {
	t.Helper()
	ops := ExtractFileOps([]llm.Message{})
	if ops.Read == nil || ops.Written == nil || ops.Edited == nil {
		t.Fatalf("ExtractFileOps returned nil maps: %+v", ops)
	}
	if got := FindCutPoint(nil, 0); got != 0 {
		t.Errorf("FindCutPoint(nil,0) = %d, want 0", got)
	}
	if got := EstimateTokens("hello"); got == 0 {
		t.Errorf("EstimateTokens(hello) = 0")
	}
	if got := TruncateForSummary("hi", 100); got != "hi" {
		t.Errorf("TruncateForSummary short passthrough = %q, want hi", got)
	}
	if got := SerializeForSummary([]llm.Message{}); got != "" {
		t.Errorf("SerializeForSummary empty = %q, want empty", got)
	}
}

func TestPureExtractFileOpsTableDriven(t *testing.T) {
	cases := []struct {
		name       string
		msgs       []llm.Message
		wantRead   map[string]bool
		wantWrote  map[string]bool
		wantEdited map[string]bool
	}{
		{
			name: "no_assistant_messages",
			msgs: []llm.Message{
				{Role: "user", Content: "hi"},
			},
			wantRead:   map[string]bool{},
			wantWrote:  map[string]bool{},
			wantEdited: map[string]bool{},
		},
		{
			name: "mixed_tool_calls",
			msgs: []llm.Message{
				{Role: "assistant", ToolCalls: []llm.ToolCall{
					{ID: "1", Function: llm.FunctionCall{Name: "read", Arguments: `{"path":"/a"}`}},
					{ID: "2", Function: llm.FunctionCall{Name: "write", Arguments: `{"path":"/b"}`}},
					{ID: "3", Function: llm.FunctionCall{Name: "edit", Arguments: `{"path":"/c"}`}},
					{ID: "4", Function: llm.FunctionCall{Name: "bash", Arguments: `{"command":"ls"}`}},
				}},
			},
			wantRead:   map[string]bool{"/a": true},
			wantWrote:  map[string]bool{"/b": true},
			wantEdited: map[string]bool{"/c": true},
		},
		{
			name: "ignores_non_assistant_roles",
			msgs: []llm.Message{
				{Role: "user", ToolCalls: []llm.ToolCall{
					{ID: "1", Function: llm.FunctionCall{Name: "read", Arguments: `{"path":"/user_path"}`}},
				}},
			},
			wantRead:   map[string]bool{},
			wantWrote:  map[string]bool{},
			wantEdited: map[string]bool{},
		},
		{
			name: "ignores_malformed_args",
			msgs: []llm.Message{
				{Role: "assistant", ToolCalls: []llm.ToolCall{
					{ID: "1", Function: llm.FunctionCall{Name: "read", Arguments: `not-json`}},
					{ID: "2", Function: llm.FunctionCall{Name: "read", Arguments: `{"other":"x"}`}},
				}},
			},
			wantRead:   map[string]bool{},
			wantWrote:  map[string]bool{},
			wantEdited: map[string]bool{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ops := ExtractFileOps(tc.msgs)
			if !mapsEqual(ops.Read, tc.wantRead) {
				t.Errorf("Read = %v, want %v", ops.Read, tc.wantRead)
			}
			if !mapsEqual(ops.Written, tc.wantWrote) {
				t.Errorf("Written = %v, want %v", ops.Written, tc.wantWrote)
			}
			if !mapsEqual(ops.Edited, tc.wantEdited) {
				t.Errorf("Edited = %v, want %v", ops.Edited, tc.wantEdited)
			}
		})
	}
}

func TestPureFindCutPointTableDriven(t *testing.T) {
	cases := []struct {
		name string
		msgs []llm.Message
		keep int
		want int
	}{
		{
			name: "keep_zero_returns_zero",
			msgs: []llm.Message{
				{Role: "user"},
			},
			keep: 0,
			want: 0,
		},
		{
			name: "empty_msgs_returns_zero",
			msgs: []llm.Message{},
			keep: 3,
			want: 0,
		},
		{
			name: "keep_larger_than_total_returns_zero",
			msgs: []llm.Message{
				{Role: "user"},
				{Role: "assistant"},
			},
			keep: 10,
			want: 0,
		},
		{
			name: "keep_one_cuts_after_first_user",
			msgs: []llm.Message{
				{Role: "user"},
				{Role: "assistant"},
				{Role: "user"},
				{Role: "assistant"},
			},
			keep: 1,
			want: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FindCutPoint(tc.msgs, tc.keep)
			if got != tc.want {
				t.Errorf("FindCutPoint = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestPureSerializeForSummaryTableDriven(t *testing.T) {
	cases := []struct {
		name        string
		msgs        []llm.Message
		mustContain []string
		mustNot     []string
	}{
		{
			name: "user_role",
			msgs: []llm.Message{
				{Role: "user", Content: "hello world"},
			},
			mustContain: []string{"[user] hello world"},
		},
		{
			name: "assistant_with_reasoning_and_tool_call",
			msgs: []llm.Message{
				{Role: "assistant", Content: "answer", Reasoning: "thinking"},
				{Role: "assistant", Content: "no think", ToolCalls: []llm.ToolCall{
					{ID: "1", Function: llm.FunctionCall{Name: "read", Arguments: `{"path":"/x"}`}},
				}},
			},
			mustContain: []string{
				"[assistant reasoning] thinking",
				"[assistant] answer",
				"[assistant tool call] read",
				`{"path":"/x"}`,
			},
		},
		{
			name: "tool_truncates_long_content",
			msgs: []llm.Message{
				{Role: "tool", ToolCallID: "c1", Content: strings.Repeat("z", 5000)},
			},
			mustContain: []string{"[tool result c1]"},
			mustNot:     []string{},
		},
		{
			name: "system_role_skipped",
			msgs: []llm.Message{
				{Role: "system", Content: "should not appear"},
			},
			mustNot: []string{"should not appear"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := SerializeForSummary(tc.msgs)
			for _, want := range tc.mustContain {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q in %q", want, out)
				}
			}
			for _, banned := range tc.mustNot {
				if strings.Contains(out, banned) {
					t.Errorf("output contains banned %q in %q", banned, out)
				}
			}
		})
	}
}

func TestPureEstimateTokensTableDriven(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"a", 1},
		{"abcd", 1},
		{"abcde", 2},
		{"hello world", 3},
		{strings.Repeat("x", 8), 2},
		{strings.Repeat("x", 400), 100},
	}

	for _, tc := range cases {
		got := EstimateTokens(tc.input)
		if got != tc.want {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestPureTruncateForSummaryTableDriven(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		max         int
		wantEqual   bool
		wantEqualV  string
		mustContain string
	}{
		{
			name:       "empty_passthrough",
			input:      "",
			max:        100,
			wantEqual:  true,
			wantEqualV: "",
		},
		{
			name:       "shorter_than_max_keep_unchanged",
			input:      "short",
			max:        100,
			wantEqual:  true,
			wantEqualV: "short",
		},
		{
			name:       "exactly_max_keep_unchanged",
			input:      "hello world!",
			max:        12,
			wantEqual:  true,
			wantEqualV: "hello world!",
		},
		{
			name:        "long_truncated_with_marker",
			input:       strings.Repeat("a", 1000),
			max:         50,
			mustContain: " more characters truncated]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TruncateForSummary(tc.input, tc.max)
			if tc.wantEqual && got != tc.wantEqualV {
				t.Errorf("got %q, want %q", got, tc.wantEqualV)
			}
			if tc.mustContain != "" && !strings.Contains(got, tc.mustContain) {
				t.Errorf("output missing %q: %q", tc.mustContain, got)
			}
		})
	}
}

func mapsEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
