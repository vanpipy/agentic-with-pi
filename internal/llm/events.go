package llm

type StreamEvent interface {
	streamEvent()
}

type EventTextDelta struct{ Text string }
type EventThinkingStart struct{}
type EventThinkingDelta struct{ Text string }
type EventThinkingEnd struct{}
type EventThinkingSignature struct{ Signature string }
type EventToolStart struct{ ID, Name string }
type EventToolDelta struct{ ID, JSON string }
type EventToolEnd struct{ ID string }
type EventToolUseSignature struct{ ID, Signature string }

type EventUsage struct {
	InputTokens         int
	OutputTokens        int
	CacheReadTokens     int
	CacheCreationTokens int
}

type EventCompaction struct {
	Trigger   string
	PreTokens int
	Native    bool
}

type EventRetryRollback struct{ Attempt, Max int }
type EventFinish struct{ Reason FinishReason }

type EventErr struct {
	Err            error
	RetryAfterSecs int
}

type EventSessionID struct{ ID string }

func (EventTextDelta) streamEvent()         {}
func (EventThinkingStart) streamEvent()     {}
func (EventThinkingDelta) streamEvent()     {}
func (EventThinkingEnd) streamEvent()       {}
func (EventThinkingSignature) streamEvent() {}
func (EventToolStart) streamEvent()         {}
func (EventToolDelta) streamEvent()         {}
func (EventToolEnd) streamEvent()           {}
func (EventToolUseSignature) streamEvent()  {}
func (EventUsage) streamEvent()             {}
func (EventCompaction) streamEvent()        {}
func (EventRetryRollback) streamEvent()     {}
func (EventFinish) streamEvent()            {}
func (EventErr) streamEvent()               {}
func (EventSessionID) streamEvent()         {}
