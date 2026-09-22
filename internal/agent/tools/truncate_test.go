package tools

import (
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
