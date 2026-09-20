package tui

import "time"

type role int

const (
	RoleUser role = iota
	RoleAssistant
	RoleTool
	RoleObserve
	RoleError
	RoleSystem
	RoleThinking
)

var (
	roleUser      = RoleUser
	roleAssistant = RoleAssistant
	roleTool      = RoleTool
	roleObserve   = RoleObserve
	roleError     = RoleError
	roleSystem    = RoleSystem
	roleThinking  = RoleThinking
)

type chatMsg struct {
	role      role
	text      string
	toolCalls []toolCallInline
	duration  time.Duration
	usage     *msgUsage
	promptNum int
	collapsed bool
}

type toolCallInline struct {
	name string
	args string
}

type msgUsage struct {
	prompt     int
	completion int
	total      int
}