package bounds

import "testing"

// The point of this package is that two surfaces cannot drift. These tests are the
// cheap insurance that the shared numbers are the ones the surfaces used before, so
// the refactor is provably behaviour-preserving rather than "looks the same".

func TestLimitsMatchTheirPreviousPerSurfaceValues(t *testing.T) {
	if MaxQueryLen != 1000 {
		t.Fatalf("MaxQueryLen = %d; the MCP and HTTP layers both used 1000, changing it is a behaviour change, not a refactor", MaxQueryLen)
	}
	if MaxNameLen != 200 {
		t.Fatalf("MaxNameLen = %d; the project-name cap was 200", MaxNameLen)
	}
	if AgentNotesDefault != 50 || AgentNotesMax != 500 {
		t.Fatalf("agent listing bounds changed: %d/%d (were 50/500)", AgentNotesDefault, AgentNotesMax)
	}
	// A path must be allowed to be far longer than a project name. This is the bug
	// the first cut had: it capped `path` with the name limit, so a legitimate deep
	// absolute path came back "unresolved" — indistinguishable from "never scanned".
	if MaxPathLen <= MaxNameLen {
		t.Fatalf("MaxPathLen (%d) must exceed MaxNameLen (%d) or path arguments get rejected by a name-sized limit",
			MaxPathLen, MaxNameLen)
	}
}

func TestClampNotesLimit(t *testing.T) {
	for in, want := range map[int]int{
		0:    AgentNotesDefault,
		-5:   AgentNotesDefault, // a negative is a mistake, not "unlimited"
		1:    1,
		50:   50,
		500:  500,
		501:  AgentNotesMax,
		9999: AgentNotesMax, // pulled back rather than rejected: the caller wants "a lot"
	} {
		if got := ClampNotesLimit(in); got != want {
			t.Errorf("ClampNotesLimit(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestCheckAndTooLong(t *testing.T) {
	if got := Check("query", "short", MaxQueryLen); got != "" {
		t.Errorf("a short value must pass, got %q", got)
	}
	msg := Check("query", string(make([]byte, MaxQueryLen+1)), MaxQueryLen)
	if msg == "" {
		t.Fatal("an oversized value must be refused")
	}
	// The message shape is shared so a caller cannot tell which transport answered.
	if want := TooLong("query", MaxQueryLen+1, MaxQueryLen); msg != want {
		t.Errorf("Check() = %q, want the same shape as TooLong(): %q", msg, want)
	}
	if TooLong("path", 0, 0) != "path too long: 0 bytes (max 0)" {
		t.Fatalf("unexpected message: %q", TooLong("path", 0, 0))
	}
}

// itoa is hand-rolled to keep this dependency-free; it is still a number
// formatter, so it gets the number-formatter tests, including the ones that
// usually break.
func TestItoa(t *testing.T) {
	for v, want := range map[int]string{
		0:       "0",
		1:       "1",
		9:       "9",
		10:      "10",
		-1:      "-1",
		-12345:  "-12345",
		1000000: "1000000",
	} {
		if got := itoa(v); got != want {
			t.Errorf("itoa(%d) = %q, want %q", v, got, want)
		}
	}
}
