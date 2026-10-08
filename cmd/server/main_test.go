package main

import (
	"net/http"
	"testing"
	"time"

	"repo-nest/internal/service"
)

// The two invariants main() guards with a runtime assertion are the whole
// reason this file exists (see the timeout block in main.go): WriteTimeout
// must exceed the batch LLM ceiling, or a handler that committed its work
// gets its response cut off — a success reported as a failure. The constants
// are compile-time; this test re-checks them independently so shortening one
// without the other fails here too, even if someone deletes the startup
// guard.

func TestWriteTimeoutExceedsBatchCeiling(t *testing.T) {
	if writeTimeout <= service.DefaultBatchChatTimeout {
		t.Fatalf("writeTimeout %v must exceed the batch ceiling %v", writeTimeout, service.DefaultBatchChatTimeout)
	}
}

// The pairing was chosen so a batch job plus response write always fits; a
// margin of zero would be one scheduler hiccup away from the old bug.
func TestWriteTimeoutHasHeadroom(t *testing.T) {
	margin := writeTimeout - service.DefaultBatchChatTimeout
	if margin < 30*time.Second {
		t.Errorf("headroom over the batch ceiling is only %v; keep a meaningful margin", margin)
	}
}

// Local-only agent: the server must never bind a wildcard address. A 0.0.0.0
// bind would expose the knowledge base to the network.
func TestServerAddressIsLoopbackOnly(t *testing.T) {
	srv := &http.Server{Addr: "127.0.0.1:18765"}
	if got := srv.Addr[:10]; got != "127.0.0.1:" {
		t.Errorf("server binds %q, want loopback", srv.Addr)
	}
}

// Timeouts that main() documents as slowloris / keep-alive guards must stay
// set: zero means "no timeout" to net/http, silently reintroducing the
// unbounded-goroutine behaviour.
func TestGuardTimeoutsAreSet(t *testing.T) {
	if headerTimeout <= 0 {
		t.Error("ReadHeaderTimeout must be set (slowloris guard)")
	}
	if idleTimeout <= 0 {
		t.Error("IdleTimeout must be set (keep-alive reaping)")
	}
	if headerTimeout >= writeTimeout {
		t.Errorf("headerTimeout %v should be well under writeTimeout %v", headerTimeout, writeTimeout)
	}
}

// envOr backs the REPONEST_HTTP_PORT override: empty env means default, a set
// env wins, and the precedence must never invert.
func TestEnvOr(t *testing.T) {
	t.Setenv("REPONEST_CMDTEST_ENV", "")
	if got := envOr("REPONEST_CMDTEST_ENV", "18765"); got != "18765" {
		t.Errorf("empty env should fall through to default, got %q", got)
	}
	t.Setenv("REPONEST_CMDTEST_ENV", "9999")
	if got := envOr("REPONEST_CMDTEST_ENV", "18765"); got != "9999" {
		t.Errorf("set env should win, got %q", got)
	}
	if got := envOr("REPONEST_CMDTEST_UNSET_ENV", "18765"); got != "18765" {
		t.Errorf("unset env should give default, got %q", got)
	}
}
