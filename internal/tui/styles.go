package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

type palette struct {
	user     color.Color
	userText color.Color
	userBg   color.Color
	ai       color.Color
	aiText   color.Color
	tool     color.Color
	fileLink color.Color
	dim      color.Color
	accent   color.Color
	system   color.Color
	success  color.Color
	warning  color.Color
	error    color.Color
	info     color.Color
	border   color.Color
}

var c = palette{
	user:     lipgloss.Color("#8ab4f8"),
	userText: lipgloss.Color("#f5f5ff"),
	userBg:   lipgloss.Color("#232832"),
	ai:       lipgloss.Color("#81c784"),
	aiText:   lipgloss.Color("#dcdcd7"),
	tool:     lipgloss.Color("#787878"),
	fileLink: lipgloss.Color("#b4c8ff"),
	dim:      lipgloss.Color("#505050"),
	accent:   lipgloss.Color("#ba8bff"),
	system:   lipgloss.Color("#ffaadc"),
	success:  lipgloss.Color("#64c864"),
	warning:  lipgloss.Color("#ffc864"),
	error:    lipgloss.Color("#ff6464"),
	info:     lipgloss.Color("#8cb4ff"),
	border:   lipgloss.Color("#64646e"),
}

var (
	headerBar = lipgloss.NewStyle().
			Background(c.accent).
			Foreground(lipgloss.Color("#000000")).
			Bold(true)

	statusOK = lipgloss.NewStyle().Foreground(c.success)
	statusErr = lipgloss.NewStyle().Foreground(c.error).Bold(true)
	statusWarn = lipgloss.NewStyle().Foreground(c.warning)
	statusSpin = lipgloss.NewStyle().Foreground(c.accent)

	helpFooter = lipgloss.NewStyle().Foreground(c.dim)

	userPromptNum = lipgloss.NewStyle().Foreground(c.accent).Bold(true)
	userPromptArrow = lipgloss.NewStyle().Foreground(c.accent)
	userPromptText = lipgloss.NewStyle().Foreground(c.userText)

	aiPrefix = lipgloss.NewStyle().Foreground(c.ai)
	aiText = lipgloss.NewStyle().Foreground(c.aiText)
	aiThinking = lipgloss.NewStyle().Foreground(c.dim).Italic(true)

	toolPrefix = lipgloss.NewStyle().Foreground(c.tool).Bold(true)
	toolName = lipgloss.NewStyle().Foreground(c.accent)

	observePrefix = lipgloss.NewStyle().Foreground(c.dim)
	systemPrefix = lipgloss.NewStyle().Foreground(c.system)
	errorPrefix = lipgloss.NewStyle().Foreground(c.error).Bold(true)

	durationHint = lipgloss.NewStyle().Foreground(c.dim)
	tokenHint = lipgloss.NewStyle().Foreground(c.dim)
	fileLinkStyle = lipgloss.NewStyle().Foreground(c.fileLink).Underline(true)

	thinkingCollapsed = lipgloss.NewStyle().Foreground(c.dim).Italic(true)
	thinkingExpanded = lipgloss.NewStyle().Foreground(c.dim).Italic(true)

	autocompleteHeader = lipgloss.NewStyle().Foreground(c.dim)
	autocompleteCursor = lipgloss.NewStyle().Foreground(c.accent)
	autocompleteCategory = lipgloss.NewStyle().Foreground(c.system)
)