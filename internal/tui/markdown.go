package tui

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
)

const mdRendererCacheCap = 8

var (
	mdRenderersMu    sync.Mutex
	mdRenderers      = make(map[int]*glamour.TermRenderer)
	mdRenderersOrder []int
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
	mdRenderersOrder = append(mdRenderersOrder, width)
	if len(mdRenderersOrder) > mdRendererCacheCap {
		evict := mdRenderersOrder[0]
		mdRenderersOrder = mdRenderersOrder[1:]
		delete(mdRenderers, evict)
	}
	return r
}

func resetMdRenderersForTest() {
	mdRenderersMu.Lock()
	defer mdRenderersMu.Unlock()
	for k := range mdRenderers {
		delete(mdRenderers, k)
	}
	mdRenderersOrder = nil
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
