// Package hook carries what an agent's tool-use hook hands weir, in a form
// that does not depend on which agent it came from.
package hook

import "io"

// Request is one tool call, reduced to what weir judges by. Every agent's own
// hook format is turned into this before any judging happens, so the judging
// never has to know which agent it is serving.
type Request struct {
	// Cwd is the directory the agent was working in.
	Cwd string
	// Tool is the agent's name for the tool being called (e.g. "Bash").
	Tool string
	// Command is the command line the tool was asked to run. It is empty for
	// a tool that runs no command.
	Command string
}

// Adapter translates one agent's hook format. weir speaks to an agent only
// through this, so supporting another agent adds an Adapter and touches
// nothing that judges.
type Adapter interface {
	// Decode reads the agent's hook input. It fails when the input is not
	// that agent's format — the caller has to stop then, not guess.
	Decode(r io.Reader) (Request, error)
	// WriteDenial writes the agent's form of "do not run this", carrying
	// reason. This is weir's only answer: it never writes an approval, so
	// the agent's own permission check still runs on everything weir lets
	// past.
	WriteDenial(w io.Writer, reason string) error
}
