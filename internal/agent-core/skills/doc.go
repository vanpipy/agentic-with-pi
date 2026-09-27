// Package skills discovers, parses, and resolves user-defined slash-command
// skills loaded from the user's ~/.awp/skills, the project's .awp/skills, and
// the project's agents/skills directories.
//
// A skill is a markdown file at <root>/<name>/SKILL.md with a YAML-style
// frontmatter:
//
//	---
//	name: <name>
//	description: <one-liner>
//	allowed-tools: <comma-separated>
//	---
//
//	<body markdown>
//
// LoadForCwd walks the three roots, parses every discovered SKILL.md, and
// resolves duplicate names with project (and agents) winning over global.
//
// ResolveInvocation maps "/<name> ..." input onto a registered skill using
// longest-name-prefix matching. Quoted arguments are unquoted.
package skills
