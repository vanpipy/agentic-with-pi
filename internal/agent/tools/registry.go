package tools

import "github.com/vanpiyp/awp/internal/agent"

// All returns the default toolset registered into the agent. New tools
// only need to be added here; main.go stays a single loop.
func All(cwd string) []agent.Tool {
	return []agent.Tool{
		ReadFile(cwd, FileOptions{}),
		WriteFile(cwd),
		EditFile(cwd),
		Bash(cwd, BashOptions{}),
		Grep(cwd, FileOptions{}),
		Find(cwd, FileOptions{}),
		Ls(cwd, FileOptions{}),
		InvalidTool(),
	}
}
