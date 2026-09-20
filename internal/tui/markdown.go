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
		return softWrap(text, width)
	}

	out, err := r.Render(text)
	if err != nil {
		return softWrap(text, width)
	}

	out = strings.TrimRight(out, "\n")
	if width <= 0 {
		return out
	}
	return softWrap(out, width)
}

func softWrap(text string, width int) string {
	if width <= 0 {
		return text
	}
	return ansi.Wordwrap(text, width, "")
}

func RenderMarkdownForTest(text string, width int) string {
	return renderMarkdownBody(text, width)
}