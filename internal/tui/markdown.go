package tui

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/x/ansi"
)

var (
	mdRenderer   *glamour.TermRenderer
	mdRendererMu sync.Mutex
)

func getMdRenderer() *glamour.TermRenderer {
	mdRendererMu.Lock()
	defer mdRendererMu.Unlock()
	if mdRenderer == nil {
		r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle(styles.NoTTYStyle),
			glamour.WithWordWrap(0),
			glamour.WithEmoji(),
		)
		if err == nil {
			mdRenderer = r
		}
	}
	return mdRenderer
}

func renderMarkdownBody(text string, width int) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	r := getMdRenderer()
	if r == nil {
		return wrapText(text, width)
	}

	out, err := r.Render(text)
	if err != nil {
		return wrapText(text, width)
	}

	out = strings.TrimRight(out, "\n")

	lines := strings.Split(out, "\n")
	if width <= 0 {
		return strings.Join(lines, "\n")
	}

	var b strings.Builder
	for _, line := range lines {
		if ansi.StringWidth(line) > width {
			b.WriteString(wrapText(line, width))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// RenderMarkdownForTest exposes renderMarkdownBody to external tests.
func RenderMarkdownForTest(text string, width int) string {
	return renderMarkdownBody(text, width)
}