package llm

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleSystem    Role = "system"
)

type ContentBlock interface {
	contentBlock()
	CacheEligible() bool
}

type ContentText struct {
	Text         string
	CacheControl *CacheControl
}

func (ContentText) contentBlock()       {}
func (ContentText) CacheEligible() bool { return true }

type ContentImage struct {
	MediaType string
	Data      string
}

func (ContentImage) contentBlock()       {}
func (ContentImage) CacheEligible() bool { return false }

type ContentToolUse struct {
	ID        string
	Name      string
	Input     []byte
	Signature string
}

func (ContentToolUse) contentBlock()       {}
func (ContentToolUse) CacheEligible() bool { return false }

type ContentToolResult struct {
	ToolUseID string
	Content   []ContentBlock
	IsError   bool
}

func (ContentToolResult) contentBlock()       {}
func (ContentToolResult) CacheEligible() bool { return false }

type ContentThinking struct {
	Text      string
	Signature string
}

func (ContentThinking) contentBlock()       {}
func (ContentThinking) CacheEligible() bool { return false }
