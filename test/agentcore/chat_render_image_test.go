package agentcore_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/tui"
)

func TestRenderMsgAssistantMarkdownImageProducesPlaceholder(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "![alt](https://example.com/img.png)",
	}, 80)
	if len(lines) == 0 {
		t.Fatal("expected at least one line for markdown image text")
	}
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "▣ image:") {
		t.Errorf("expected image placeholder '▣ image:' in output, got %q", combined)
	}
}

func TestRenderMsgAssistantMarkdownImageEmptyAlt(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "![](https://example.com/img.png)",
	}, 80)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "▣ image:") {
		t.Errorf("expected image placeholder for empty alt, got %q", combined)
	}
}

func TestRenderMsgAssistantMarkdownImageInfersTypeFromExtension(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"![](https://example.com/foo.png)", "▣ image: png"},
		{"![](https://example.com/foo.jpg)", "▣ image: jpg"},
		{"![](https://example.com/foo.jpeg)", "▣ image: jpeg"},
		{"![](https://example.com/foo.gif)", "▣ image: gif"},
		{"![](https://example.com/foo.webp)", "▣ image: webp"},
		{"![](https://example.com/foo.svg)", "▣ image: svg"},
	}
	for _, tc := range cases {
		lines := tui.RenderMsgForTest(tui.ChatMsg{
			Role: tui.RoleAssistant,
			Text: tc.text,
		}, 80)
		combined := strings.Join(lines, "\n")
		if !strings.Contains(combined, tc.want) {
			t.Errorf("for text=%q expected %q in output, got %q", tc.text, tc.want, combined)
		}
	}
}

func TestRenderMsgAssistantMarkdownImageDataURL(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "![inline](data:image/png;base64,iVBORw0KGgo=)",
	}, 80)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "▣ image: png") {
		t.Errorf("expected '▣ image: png' for data URL, got %q", combined)
	}
}

func TestRenderMsgAssistantMarkdownImageDataURLJpeg(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "![pic](data:image/jpeg;base64,/9j/4AAQ)",
	}, 80)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "▣ image: jpeg") {
		t.Errorf("expected '▣ image: jpeg' for data URL, got %q", combined)
	}
}

func TestRenderMsgAssistantMarkdownImageEmptyURLSkipped(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "before ![alt]() after",
	}, 80)
	combined := strings.Join(lines, "\n")
	if strings.Contains(combined, "▣ image:") {
		t.Errorf("empty URL should not produce placeholder, got %q", combined)
	}
	if !strings.Contains(combined, "before") {
		t.Errorf("expected 'before' preserved, got %q", combined)
	}
	if !strings.Contains(combined, "after") {
		t.Errorf("expected 'after' preserved, got %q", combined)
	}
}

func TestRenderMsgAssistantMarkdownImageWithSurroundingText(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "before\n\n![alt](https://example.com/img.png)\n\nafter",
	}, 80)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "before") {
		t.Errorf("expected 'before' preserved, got %q", combined)
	}
	if !strings.Contains(combined, "after") {
		t.Errorf("expected 'after' preserved, got %q", combined)
	}
	if !strings.Contains(combined, "▣ image:") {
		t.Errorf("expected image placeholder, got %q", combined)
	}
}

func TestRenderMsgAssistantMultipleMarkdownImages(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "![a](https://example.com/a.png) and ![b](https://example.com/b.jpg)",
	}, 80)
	combined := strings.Join(lines, "\n")
	if strings.Count(combined, "▣ image:") < 2 {
		t.Errorf("expected 2 image placeholders, got count=%d, output=%q",
			strings.Count(combined, "▣ image:"), combined)
	}
}

func TestRenderMsgAssistantImageInsideCodeBlockPreserved(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "```bash\n![alt](https://example.com/img.png)\n```",
	}, 80)
	combined := strings.Join(lines, "\n")
	if strings.Contains(combined, "▣ image:") {
		t.Errorf("image inside code fence should not be extracted, got %q", combined)
	}
	if !strings.Contains(combined, "![alt]") {
		t.Errorf("image syntax inside code fence should be preserved as raw text, got %q", combined)
	}
}

func TestRenderMsgAssistantHalfOpenedImagePreserved(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "text ![alt without closing paren more text",
	}, 80)
	combined := strings.Join(lines, "\n")
	if strings.Contains(combined, "▣ image:") {
		t.Errorf("half-opened image syntax should not extract, got %q", combined)
	}
}

func TestRenderMsgAssistantImageInsidePlanBlockUntouched(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "```plan\n![alt](https://example.com/img.png)\n```",
	}, 80)
	combined := strings.Join(lines, "\n")
	if strings.Contains(combined, "▣ image:") {
		t.Errorf("image inside plan block should not be a placeholder, got %q", combined)
	}
}

func TestRenderMsgAssistantImageInsideDiffBlockUntouched(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "```diff\n+ ![alt](https://example.com/img.png)\n```",
	}, 80)
	combined := strings.Join(lines, "\n")
	if strings.Contains(combined, "▣ image:") {
		t.Errorf("image inside diff block should not be a placeholder, got %q", combined)
	}
}

func TestRenderMsgAssistantImagePlaceholderHasAnsiEscape(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "![a](https://example.com/img.png)",
	}, 80)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "▣ image:") {
		t.Fatalf("expected image placeholder, got %q", combined)
	}
	if !strings.Contains(combined, "\x1b[") {
		t.Errorf("placeholder should have dim ANSI style, got %q", combined)
	}
}

func TestRenderMsgAssistantImageAndPlanBlocksCoexist(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "intro\n\n![a](https://example.com/img.png)\n\n```plan\n- step\n```",
	}, 80)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "intro") {
		t.Errorf("expected 'intro' preserved, got %q", combined)
	}
	if !strings.Contains(combined, "▣ image:") {
		t.Errorf("expected image placeholder, got %q", combined)
	}
	if !strings.Contains(combined, "step") {
		t.Errorf("expected plan content 'step' preserved, got %q", combined)
	}
	if !strings.Contains(combined, "╭") {
		t.Errorf("expected plan block top border, got %q", combined)
	}
}

func TestRenderMsgAssistantImageAndDiffBlocksCoexist(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "intro\n\n![a](https://example.com/img.png)\n\n```diff\n+ added\n- removed\n```",
	}, 80)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "intro") {
		t.Errorf("expected 'intro' preserved, got %q", combined)
	}
	if !strings.Contains(combined, "▣ image:") {
		t.Errorf("expected image placeholder, got %q", combined)
	}
	if !strings.Contains(combined, "added") {
		t.Errorf("expected diff content 'added' preserved, got %q", combined)
	}
	if !strings.Contains(combined, "╭") {
		t.Errorf("expected diff block top border, got %q", combined)
	}
}

func TestRenderMsgAssistantWithoutImageNoPlaceholder(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "just a normal reply with **bold** and a list:\n\n- a\n- b",
	}, 80)
	if len(lines) == 0 {
		t.Fatal("expected at least one line for plain assistant text")
	}
	combined := strings.Join(lines, "\n")
	if strings.Contains(combined, "▣ image:") {
		t.Errorf("plain text should not contain image placeholder, got %q", combined)
	}
}

func TestRenderMsgAssistantImageDoesNotLeakMarkdownSyntax(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "before ![alt](https://example.com/img.png) after",
	}, 80)
	combined := strings.Join(lines, "\n")
	if strings.Contains(combined, "![alt]") {
		t.Errorf("original markdown image syntax should be stripped from extracted image, got %q", combined)
	}
}

func TestRenderMsgAssistantBareDataURLProducesPlaceholder(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "see below\n\ndata:image/png;base64,iVBORw0KGgo=\n\ndone",
	}, 80)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "▣ image: png") {
		t.Errorf("expected '▣ image: png' for bare data URL, got %q", combined)
	}
	if !strings.Contains(combined, "see below") {
		t.Errorf("expected 'see below' preserved, got %q", combined)
	}
	if !strings.Contains(combined, "done") {
		t.Errorf("expected 'done' preserved, got %q", combined)
	}
}

func TestRenderMsgAssistantImageBetweenParagraphs(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "first paragraph\n\n![snap](https://example.com/snap.png)\n\nsecond paragraph",
	}, 80)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "first paragraph") {
		t.Errorf("expected 'first paragraph' preserved, got %q", combined)
	}
	if !strings.Contains(combined, "second paragraph") {
		t.Errorf("expected 'second paragraph' preserved, got %q", combined)
	}
	if !strings.Contains(combined, "▣ image: png") {
		t.Errorf("expected image placeholder, got %q", combined)
	}
}

func TestRenderMsgAssistantImageOnItsOwnLineIsRecognized(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "![solo](https://example.com/solo.png)",
	}, 80)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "▣ image: png") {
		t.Errorf("expected '▣ image: png' for standalone image line, got %q", combined)
	}
}

func TestRenderMsgAssistantImageURLWithQueryString(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "![a](https://example.com/foo.png?w=100&h=200)",
	}, 80)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "▣ image: png") {
		t.Errorf("expected '▣ image: png' for URL with query string, got %q", combined)
	}
}

func TestRenderMsgAssistantImageInsideNestedCodeFenceIgnored(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "intro\n\n```\nsome plain code\n![alt](https://example.com/img.png)\nstill code\n```\n\noutro",
	}, 80)
	combined := strings.Join(lines, "\n")
	if strings.Contains(combined, "▣ image:") {
		t.Errorf("image inside ``` fence should not be extracted, got %q", combined)
	}
	if !strings.Contains(combined, "intro") {
		t.Errorf("expected 'intro' preserved, got %q", combined)
	}
	if !strings.Contains(combined, "outro") {
		t.Errorf("expected 'outro' preserved, got %q", combined)
	}
}

func TestRenderMsgAssistantImageWithEscapedBracketInAlt(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: `![alt with ] inside](https://example.com/img.png)`,
	}, 80)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "▣ image: png") {
		t.Errorf("expected '▣ image: png' for alt with escaped bracket, got %q", combined)
	}
}

func TestRenderMsgAssistantImageStreamingAlsoProducesPlaceholder(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SetSizeForTest(80, 40)
	m.AppendStreamForTest("![a](https://example.com/img.png)")
	combined := m.ContentForTest()
	if !strings.Contains(combined, "▣ image: png") {
		t.Errorf("streaming markdown image should produce placeholder, got %q", combined)
	}
}
