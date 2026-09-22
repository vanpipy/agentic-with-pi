package tui

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
)

var (
	mdRenderersMu sync.Mutex
	mdRenderers   = make(map[int]*glamour.TermRenderer)
)

func getMdRenderer(width int) *glamour.TermRenderer {
	mdRenderersMu.Lock()
	defer mdRenderersMu.Unlock()
	if r, ok := mdRenderers[width]; ok {
		return r
	}
	options := []glamour.TermRendererOption{
		glamour.WithStandardStyle(styles.NoTTYStyle),
		glamour.WithEmoji(),
	}
	if width > 0 {
		options = append(options, glamour.WithWordWrap(width))
	}
	r, err := glamour.NewTermRenderer(options...)
	if err != nil {
		return nil
	}
	mdRenderers[width] = r
	return r
}

func renderMarkdownBody(text string, width int) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	r := getMdRenderer(width)
	if r == nil {
		return text
	}
	out, err := r.Render(text)
	if err != nil {
		return text
	}
	return strings.TrimRight(out, "\n")
}

func RenderMarkdownForTest(text string, width int) string {
	return renderMarkdownBody(text, width)
}
