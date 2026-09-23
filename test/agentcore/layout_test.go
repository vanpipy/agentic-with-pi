package agentcore_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/vanpiyp/awp/internal/tui"
)

func TestTextareaGrowthPushesBodyDown(t *testing.T) {
	m := tui.NewModelForTest()
	out, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = out.(*tui.Model)

	for _, r := range "first line" {
		out, _ = m.Update(tea.KeyPressMsg{Text: string(r)})
		m = out.(*tui.Model)
	}

	v := m.View()
	plain := stripANSI(v.Content)
	lines := strings.Split(plain, "\n")
	if len(lines) != 30 {
		t.Errorf("View should be 30 lines, got %d", len(lines))
	}
}

func TestTextareaBodyShrinksAsInputGrows(t *testing.T) {
	m := tui.NewModelForTest()
	out, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = out.(*tui.Model)

	countInputLines := func() int {
		v := m.View()
		plain := stripANSI(v.Content)
		lines := strings.Split(plain, "\n")
		inputStart := -1
		for i, line := range lines {
			if strings.HasPrefix(line, "┃ ") || strings.Contains(line, "type a prompt") {
				inputStart = i
				break
			}
		}
		if inputStart < 0 {
			return 0
		}
		end := len(lines)
		for j := inputStart; j < len(lines); j++ {
			if !strings.HasPrefix(lines[j], "┃ ") {
				end = j
				break
			}
		}
		return end - inputStart
	}

	for _, r := range "first line" {
		out, _ = m.Update(tea.KeyPressMsg{Text: string(r)})
		m = out.(*tui.Model)
	}
	oneLine := countInputLines()
	if oneLine != 1 {
		t.Errorf("1 line of text should render 1 input row, got %d", oneLine)
	}

	out, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	m = out.(*tui.Model)
	for _, r := range "second line" {
		out, _ = m.Update(tea.KeyPressMsg{Text: string(r)})
		m = out.(*tui.Model)
	}
	out, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	m = out.(*tui.Model)
	for _, r := range "third line" {
		out, _ = m.Update(tea.KeyPressMsg{Text: string(r)})
		m = out.(*tui.Model)
	}
	out, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	m = out.(*tui.Model)
	for _, r := range "fourth line" {
		out, _ = m.Update(tea.KeyPressMsg{Text: string(r)})
		m = out.(*tui.Model)
	}
	fourLine := countInputLines()
	if fourLine != 4 {
		t.Errorf("4 lines should render 4 input rows, got %d", fourLine)
	}

	v := m.View()
	plain := stripANSI(v.Content)
	lines := strings.Split(plain, "\n")
	if len(lines) != 30 {
		t.Errorf("View should still fit 30 lines after textarea grew, got %d", len(lines))
	}
}

func TestAutocompletePopupReservesSpace(t *testing.T) {
	for _, h := range []int{16, 20, 30} {
		m := tui.NewModelForTest()
		out, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: h})
		m = out.(*tui.Model)

		for _, r := range "/ne" {
			out, _ = m.Update(tea.KeyPressMsg{Text: string(r)})
			m = out.(*tui.Model)
		}

		v := m.View()
		plain := stripANSI(v.Content)
		lines := strings.Split(plain, "\n")
		if len(lines) != h {
			t.Errorf("height=%d: View should be %d lines, got %d", h, h, len(lines))
		}
	}
}

func TestTextareaMultiLineShrinksBody(t *testing.T) {
	m := tui.NewModelForTest()
	out, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = out.(*tui.Model)

	for _, r := range "first line" {
		out, _ = m.Update(tea.KeyPressMsg{Text: string(r)})
		m = out.(*tui.Model)
	}
	out, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	m = out.(*tui.Model)

	for _, r := range "second line" {
		out, _ = m.Update(tea.KeyPressMsg{Text: string(r)})
		m = out.(*tui.Model)
	}
	out, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	m = out.(*tui.Model)

	for _, r := range "third line" {
		out, _ = m.Update(tea.KeyPressMsg{Text: string(r)})
		m = out.(*tui.Model)
	}
	out, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	m = out.(*tui.Model)

	for _, r := range "fourth line" {
		out, _ = m.Update(tea.KeyPressMsg{Text: string(r)})
		m = out.(*tui.Model)
	}

	v := m.View()
	plain := stripANSI(v.Content)
	lines := strings.Split(plain, "\n")
	t.Logf("after 4 lines (no submit): View has %d lines", len(lines))
	for i, line := range lines {
		t.Logf("[%2d] %s", i, line)
	}
}
