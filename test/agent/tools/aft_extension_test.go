package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent/tools"
)

func writeFakeAftEchoing(t *testing.T, dir string) string {
	t.Helper()
	bin := filepath.Join(dir, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  cmd=$(printf '%s' \"$line\" | sed -n 's/.*\"command\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"%s\",\"success\":true,\"command\":\"%s\",\"output\":\"ok\"}\\n' \"$id\" \"$cmd\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestAftExtensionRoutesThroughBackend(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  rest=$(printf '%s' \"$line\" | sed 's/.*\"command\":\"[^\"]*\",\\(.*\\)}$/\\1/')\n  printf '{\"id\":\"%s\",\"success\":true,\"command\":\"read\",%s}\\n' \"$id\" \"$rest\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	read := tools.ReadAftForTest(backend)
	got, err := read.Invoke(context.Background(), `{"path":"/tmp/x"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"command":"read"`) {
		t.Errorf("read wrapper did not route to read command: %q", got)
	}
	if !strings.Contains(got, `"path":"/tmp/x"`) {
		t.Errorf("read wrapper dropped params: %q", got)
	}
}

func TestAftExtensionEveryWrapperPassesThrough(t *testing.T) {
	tmp := t.TempDir()
	bin := writeFakeAftEchoing(t, tmp)
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	cases := []struct {
		name string
		fn   func() (string, error)
	}{
		{"read", func() (string, error) {
			return tools.ReadAftForTest(backend).Invoke(context.Background(), `{"path":"/a"}`)
		}},
		{"write", func() (string, error) {
			return tools.WriteAftForTest(backend).Invoke(context.Background(), `{"path":"/a","content":"hi"}`)
		}},
		{"edit_match", func() (string, error) {
			return tools.EditAftForTest(backend).Invoke(context.Background(), `{"path":"/a","old_string":"x","new_string":"y"}`)
		}},
		{"bash", func() (string, error) {
			return tools.BashAftForTest(backend).Invoke(context.Background(), `{"command":"echo hi"}`)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := tc.fn()
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if !strings.Contains(out, `"command":"`+tc.name+`"`) {
				t.Errorf("%s wrapper did not route to %s command: %q", tc.name, tc.name, out)
			}
		})
	}
}
