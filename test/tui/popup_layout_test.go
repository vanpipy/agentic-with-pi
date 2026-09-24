package tui_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/tui"
)

func TestView_AutocompleteVisible_DoesNotCropChatTop(t *testing.T) {
	m := tui.NewModelForTest()
	m.SetSizeForTest(80, 24)

	earliest := "EARLIEST_MARKER_aaa"
	latest := "LATEST_MARKER_zzz"
	for i := 0; i < 10; i++ {
		m.EnterForTest(strings.Repeat("long message text here ", 6))
	}
	m.EnterForTest(earliest)
	for i := 0; i < 5; i++ {
		m.EnterForTest(strings.Repeat("long message text here ", 6))
	}
	m.EnterForTest(latest)

	m.InputAppendForTest("/")
	m.AutocompleteRefreshForTest()
	m.LayoutForTest()

	if !m.AutocompleteVisibleForTest() {
		t.Fatalf("popup not visible after setQuery(/); height=%d", m.InputHeightForTest())
	}

	v := m.ViewForTest()
	lines := strings.Split(v, "\n")
	if len(lines) != 24 {
		t.Fatalf("View split elements: %d (want 24)", len(lines))
	}

	body := strings.Join(lines[2:20], "\n")
	if !strings.Contains(body, earliest) {
		t.Fatalf("chat top was cropped: body=%q", body)
	}
}

func TestView_NoPopup_NormalLayout(t *testing.T) {
	m := tui.NewModelForTest()
	m.SetSizeForTest(80, 24)

	for i := 0; i < 10; i++ {
		m.EnterForTest(strings.Repeat("long message text here ", 6))
	}

	m.LayoutForTest()

	v := m.ViewForTest()
	lines := strings.Split(v, "\n")
	if len(lines) != 24 {
		t.Fatalf("View split elements: %d (want 24)", len(lines))
	}
}
