// Package bounds holds the input-size limits that every consumer surface shares.
//
// Why this exists: the same limits were written out separately in the MCP layer
// and in the HTTP layer, with the HTTP comment literally saying it was "mirroring
// the MCP layer's query bound". Mirroring by hand is how two surfaces end up
// accepting different input from two callers doing the same thing — and the pair
// that drifted here was a length cap, i.e. the guard that keeps a 100 KB query
// from turning an FTS5 phrase into a full-table scan.
//
// The rule this package encodes: a limit belongs to the operation, not to the
// transport. The MCP tool, the /api/rpc binding and the REST route all reach the
// same service method, so the method (or the shared constant) decides the bound,
// and the surfaces enforce the same number rather than their own copy of it.
package bounds

// Query and text argument limits, in bytes (not runes: these guard request size,
// and the value is compared against len() of a UTF-8 string).
const (
	// MaxQueryLen bounds a search query on any surface. Beyond this, a query is a
	// paste-the-whole-note attempt, not a question.
	MaxQueryLen = 1000

	// MaxNameLen bounds a project name or path used to RESOLVE a project. It is
	// deliberately larger than MaxQueryLen: an absolute path on a deep tree is long
	// by nature, and truncating it silently would resolve the wrong project.
	MaxNameLen = 200

	// MaxPathLen bounds a workspace directory argument. Longer than a name because
	// it is an absolute path, and far shorter than any real filesystem limit so a
	// junk argument still fails fast.
	MaxPathLen = 4096
)

// Listing limits. The desktop surface is intentionally unbounded (see
// internal/app/surface.go): a loopback HTTP bridge is reachable only by the same
// user who can already open the database file, so a cap there protects nothing and
// breaks the UI's "show me everything". The agent-facing surface is bounded because
// an agent's context window, not the disk, is the scarce resource.
const (
	// AgentNotesDefault is what repotory_notes_list returns when no limit is given.
	AgentNotesDefault = 50
	// AgentNotesMax is the ceiling an agent may ask for.
	AgentNotesMax = 500
)

// ClampNotesLimit applies the agent-facing listing bound: non-positive becomes the
// default, over-max is pulled back rather than rejected (an agent asking for 10000
// wants "a lot", not an error).
func ClampNotesLimit(limit int) int {
	if limit <= 0 {
		return AgentNotesDefault
	}
	if limit > AgentNotesMax {
		return AgentNotesMax
	}
	return limit
}

// TooLong is the shared shape of an argument-length refusal, so the message a
// caller sees does not depend on which transport they arrived through.
func TooLong(what string, got, max int) string {
	return what + " too long: " + itoa(got) + " bytes (max " + itoa(max) + ")"
}

// Check returns TooLong(what, ...) when len(value) exceeds max, else "".
func Check(what, value string, max int) string {
	if len(value) > max {
		return TooLong(what, len(value), max)
	}
	return ""
}

// itoa without importing strconv into a package callers use for constants only.
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
