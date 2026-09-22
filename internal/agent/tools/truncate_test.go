package tools

import (
	"strings"
	"testing"

	"github.com/rivo/uniseg"
)

func TestTruncateMiddleShortStringUnchanged(t *testing.T) {
	got := TruncateMiddle("hello", 10)
	if got != "hello" {
		t.Errorf("TruncateMiddle(hello, 10) = %q, want hello", got)
	}
}

func TestTruncateMiddleASCIIInsertEllipsis(t *testing.T) {
	got := TruncateMiddle("hello world", 8)
	want := "hell" + "…" + "rld"
	if got != want {
		t.Errorf("TruncateMiddle(hello world, 8) = %q, want %q", got, want)
	}
}

func TestTruncateMiddleCJKCountsAsTwo(t *testing.T) {
	got := TruncateMiddle("中文字符测试", 7)
	want := "中" + "…" + "试"
	if got != want {
		t.Errorf("TruncateMiddle(中文字符测试, 7) = %q, want %q", got, want)
	}
}

func TestTruncateMiddleZeroWidthReturnsEmpty(t *testing.T) {
	if got := TruncateMiddle("anything", 0); got != "" {
		t.Errorf("TruncateMiddle(_, 0) = %q, want empty", got)
	}
}

func TestTruncateMiddleOneWidthReturnsEllipsis(t *testing.T) {
	if got := TruncateMiddle("anything", 1); got != "…" {
		t.Errorf("TruncateMiddle(_, 1) = %q, want …", got)
	}
}

func TestTruncateEndShortStringUnchanged(t *testing.T) {
	got := TruncateEnd("hello", 10)
	if got != "hello" {
		t.Errorf("TruncateEnd(hello, 10) = %q, want hello", got)
	}
}

func TestTruncateEndASCIIAddsEllipsis(t *testing.T) {
	got := TruncateEnd("hello world", 6)
	if got != "hello…" {
		t.Errorf("TruncateEnd(hello world, 6) = %q, want hello…", got)
	}
}

func TestDisplayPrefixByWidthHandlesEmoji(t *testing.T) {
	got := displayPrefixByWidth("🎉🎉🎉", 4)
	if uniseg.StringWidth(got) > 4 {
		t.Errorf("displayPrefixByWidth returned width %d > 4: %q", uniseg.StringWidth(got), got)
	}
}

func TestTruncatePathShortUnchanged(t *testing.T) {
	got := TruncatePath("/home/user/file.rs", 100)
	if got != "/home/user/file.rs" {
		t.Errorf("got %q, want unchanged", got)
	}
}

func TestTruncatePathAbsoluteKeepsMarker(t *testing.T) {
	got := TruncatePath("/home/very/long/path/to/some/deeply/nested/file.rs", 25)
	if !strings.HasPrefix(got, "/…/") {
		t.Errorf("absolute path should start with /…/, got %q", got)
	}
	if !strings.HasSuffix(got, "file.rs") {
		t.Errorf("should keep last segment, got %q", got)
	}
}

func TestTruncatePathHomePrefix(t *testing.T) {
	got := TruncatePath("~/very/long/path/to/file.rs", 20)
	if !strings.HasPrefix(got, "~/…/") {
		t.Errorf("home-relative path should start with ~/…/, got %q", got)
	}
}

func TestTruncatePathRelativePrefix(t *testing.T) {
	got := TruncatePath("./long/path/to/file.rs", 20)
	if !strings.HasPrefix(got, "./…/") {
		t.Errorf("relative path should start with ./…/, got %q", got)
	}
}

func TestTruncatePathBareName(t *testing.T) {
	got := TruncatePath("verylongfilename.rs", 12)
	if !strings.HasPrefix(got, "…/") {
		t.Errorf("bare name should start with …/, got %q", got)
	}
}

func TestTruncatePathZeroWidth(t *testing.T) {
	if got := TruncatePath("/anywhere", 0); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestTruncateCommandShortUnchanged(t *testing.T) {
	got := TruncateCommand("ls -la", 100)
	if got != "ls -la" {
		t.Errorf("got %q, want unchanged", got)
	}
}

func TestTruncateCommandKeepsTokens(t *testing.T) {
	got := TruncateCommand("git commit -m 'a very long commit message that goes on forever'", 25)
	if !strings.HasPrefix(got, "git") {
		t.Errorf("should start with first token, got %q", got)
	}
	if !strings.Contains(got, "…") {
		t.Errorf("should contain ellipsis, got %q", got)
	}
}

func TestTruncateCommandTwoTokensFallsBack(t *testing.T) {
	got := TruncateCommand("averylongcommandnamewithoutanyspaces", 10)
	if !strings.Contains(got, "…") {
		t.Errorf("two-token command should truncate with ellipsis, got %q", got)
	}
}
