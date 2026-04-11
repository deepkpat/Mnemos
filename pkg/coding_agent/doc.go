// Package coding_agent provides a coding agent implementation with tools for file operations
// and session management.
//
// The package provides tools similar to Claude Code and includes:
//
// Tools:
//   - bash: Execute shell commands
//   - read: Read file contents with optional line limits
//   - write: Write files (creates parent directories)
//   - edit: Edit files using ed-style diffs
//   - ls: List directory with sorting options
//   - grep: Search for patterns in files
//   - find: Find files by name or type
//
// Session Management:
//   - NewSession: Create a new coding session
//   - LoadSession: Load an existing session
//   - Session persistence via JSON files
//
// Usage:
//
//	ca, err := coding_agent.NewCodingAgent(&coding_agent.CodingAgentOptions{
//	    Model: model,
//	})
//	if err != nil {
//	    return err
//	}
//
//	if err := ca.Prompt(ctx, "Write a hello world program"); err != nil {
//	    return err
//	}
//
//	ca.Wait()
package coding_agent
