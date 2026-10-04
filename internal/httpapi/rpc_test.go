package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"repo-nest/internal/db"
	"repo-nest/internal/service"
)

type fakeBound struct{}

func (fakeBound) Echo(s string) string  { return s }
func (fakeBound) Add(a, b int64) int64  { return a + b }
func (fakeBound) Fail() error           { return errors.New("boom") }
func (fakeBound) Pair() (string, error) { return "ok", nil }
func (fakeBound) List() []int64         { return []int64{1, 2, 3} }

func newRPCHandler(t *testing.T) *handler {
	t.Helper()
	d, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return &handler{svc: service.New(d, "me"), bound: reflect.ValueOf(fakeBound{})}
}

func rpc(t *testing.T, h *handler, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/rpc", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.rpc(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestRPCHappyPaths(t *testing.T) {
	h := newRPCHandler(t)
	if code, out := rpc(t, h, `{"method":"Echo","args":["hi"]}`); code != 200 || out["result"] != "hi" {
		t.Fatalf("Echo: code=%d out=%v", code, out)
	}
	if code, out := rpc(t, h, `{"method":"Add","args":[2,3]}`); code != 200 || out["result"].(float64) != 5 {
		t.Fatalf("Add: code=%d out=%v", code, out)
	}
	if code, out := rpc(t, h, `{"method":"Pair"}`); code != 200 || out["result"] != "ok" {
		t.Fatalf("Pair: code=%d out=%v", code, out)
	}
	if code, out := rpc(t, h, `{"method":"List"}`); code != 200 {
		t.Fatalf("List code=%d", code)
	} else if _, ok := out["result"].([]any); !ok {
		t.Fatalf("List result not array: %v", out["result"])
	}
}

func TestRPCErrorPaths(t *testing.T) {
	h := newRPCHandler(t)
	// method returns error -> 422 with message.
	if code, out := rpc(t, h, `{"method":"Fail"}`); code != 422 || out["error"] != "boom" {
		t.Fatalf("Fail: code=%d out=%v", code, out)
	}
	// unknown method -> 404.
	if code, _ := rpc(t, h, `{"method":"Nope"}`); code != 404 {
		t.Fatalf("unknown method code=%d, want 404", code)
	}
	// bad arg type -> 400.
	if code, _ := rpc(t, h, `{"method":"Add","args":["x",2]}`); code != 400 {
		t.Fatalf("bad arg code=%d, want 400", code)
	}
	// GET not allowed -> 405.
	rec := httptest.NewRecorder()
	h.rpc(rec, httptest.NewRequest("GET", "/api/rpc", nil))
	if rec.Code != 405 {
		t.Fatalf("GET code=%d, want 405", rec.Code)
	}
}

// The bridge must never expose the Wails lifecycle hooks: a single POST
// {"method":"Shutdown"} used to close the database on the live server.
func TestRPCBlocksLifecycleMethods(t *testing.T) {
	h := newRPCHandler(t)
	for _, method := range []string{"Shutdown", "Startup", "Service"} {
		if code, out := rpc(t, h, `{"method":"`+method+`"}`); code != 403 {
			t.Fatalf("%s: code=%d out=%v, want 403", method, code, out)
		}
	}
}

// Oversized request bodies are rejected with 413 before any decoding work.
func TestRPCBodySizeLimit(t *testing.T) {
	h := newRPCHandler(t)
	big := `{"method":"Echo","args":["` + strings.Repeat("a", maxRPCBodyBytes+16) + `"]}`
	if code, _ := rpc(t, h, big); code != 413 {
		t.Fatalf("oversized body code=%d, want 413", code)
	}
}

// A method whose context parameter is omitted gets the request context, not a
// nil interface (which would panic the method on first ctx use).
func TestRPCNilContextBecomesRequestContext(t *testing.T) {
	h := newRPCHandler(t)
	if code, out := rpc(t, h, `{"method":"CtxEcho"}`); code != 200 || out["result"] == "" {
		t.Fatalf("CtxEcho: code=%d out=%v", code, out)
	}
}

func (fakeBound) CtxEcho(ctx context.Context) string {
	if ctx == nil {
		return "nil"
	}
	if ctx.Err() != nil {
		return "err"
	}
	return "ctx"
}

// A future signature drift (non-error second return) must degrade to 501, not
// panic the handler.
func TestRPCUnsupportedSignature(t *testing.T) {
	h := newRPCHandler(t)
	if code, _ := rpc(t, h, `{"method":"BadSignature"}`); code != 501 {
		t.Fatalf("BadSignature code=%d, want 501", code)
	}
}

func (fakeBound) BadSignature() (string, int) { return "x", 1 }
