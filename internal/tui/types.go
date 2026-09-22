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

const (
	StateReady State = iota
	StateStreaming
	StateError
)

func StateStreamingForTestValue() State { return StateStreaming }

type chatMsg struct {
	role      role
	text      string
	intent    string
	duration  time.Duration
	usage     *msgUsage
	promptNum int
	collapsed bool
}

type msgUsage struct {
	prompt     int
	completion int
	total      int
}