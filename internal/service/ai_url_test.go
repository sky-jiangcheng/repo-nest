package service

import (
	"strings"
	"testing"
)

// Regression guard for the localhost→127.0.0.1 fallback in aiGet/aiPostJSON.
//
// The old implementation rewrote the URL with a string replace on "://localhost:".
// Two things went wrong with that:
//   - a base URL already typed as an IP still went through the replace branch and
//     came out as the unparseable "http://127.0.0.1:127.0.0.1:1234/v1"
//   - a path or query containing "localhost" would be rewritten too
//
// The host is now swapped through net/url, so only an actual localhost hostname
// changes and the result must always stay parseable.
func TestAiLocalhostFallback(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // "" means "no fallback applies"
	}{
		{"localhost with port", "http://localhost:1234/v1/models", "http://127.0.0.1:1234/v1/models"},
		{"uppercase host", "http://LOCALHOST:1234/v1/models", "http://127.0.0.1:1234/v1/models"},
		{"no port", "http://localhost/v1/models", "http://127.0.0.1/v1/models"},
		{"query preserved", "http://localhost:1234/v1/models?a=b", "http://127.0.0.1:1234/v1/models?a=b"},
		// Already an IP: nothing to do. The old code produced the doubled host.
		{"ipv4 literal", "http://127.0.0.1:1235/v1/models", ""},
		{"ipv4 literal 1234", "http://127.0.0.1:1234/v1/models", ""},
		// Remote hosts must never be redirected to the local machine.
		{"remote host", "https://api.openai.com/v1/models", ""},
		{"lan host", "http://192.168.1.50:1234/v1/models", ""},
		// localhost appearing anywhere but the host is not a host match.
		{"localhost in path", "http://example.com/localhost/v1", ""},
		{"localhost in query", "http://example.com/v1?q=localhost", ""},
		{"localhost in path of localhost host", "http://localhost/localhost/v1", "http://127.0.0.1/localhost/v1"},
		{"subdomain is not localhost", "http://notlocalhost:1234/v1", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := aiLocalhostFallback(tc.in)
			if got != tc.want {
				t.Fatalf("aiLocalhostFallback(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if got != "" && strings.Contains(got, "127.0.0.1:127.0.0.1") {
				t.Errorf("fallback produced a doubled host: %q", got)
			}
		})
	}
}

// A base URL typed as an IP must produce usable candidates — the probe used to
// blow up on "invalid port" before it ever reached the network.
func TestListAIModels_IPBaseURLStaysParseable(t *testing.T) {
	// Port 1 is closed, so this fails at connect rather than at parse. The point
	// is that the error is the connection hint, never a malformed-URL error.
	_, err := ListAIModels("http://127.0.0.1:1/v1", "")
	if err == nil {
		t.Fatal("expected an error from a closed port")
	}
	if strings.Contains(err.Error(), "invalid port") || strings.Contains(err.Error(), "invalid URL") {
		t.Errorf("IP base URL produced a parse error instead of a network error: %v", err)
	}
}