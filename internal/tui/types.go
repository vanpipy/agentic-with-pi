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

type State int

const (
	StateReady State = iota
	StateStreaming
	StateError
)

func (s State) String() string {
	switch s {
	case StateReady:
		return "ready"
	case StateStreaming:
		return "streaming"
	case StateError:
		return "error"
	}
	return "unknown"
}

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
