package vectordb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Qdrant talks to a remote/self-hosted Qdrant server over its REST API
// (http://<host>:6333). It is used only when the user opts in via the
// vector_store config; the service falls back to Local if it is unreachable.
//
// NOTE: the wire contract here mirrors Qdrant's documented REST (PUT
// /collections/{c}, PUT /collections/{c}/points, POST /collections/{c}/points/
// search, `api-key` header). Because this package cannot exercise a live Qdrant
// in unit tests (httptest only), run ONE real smoke test against your Qdrant
// before enabling it for users.
type Qdrant struct {
	Base       string // e.g. http://localhost:6333
	APIKey     string // optional (Qdrant may run with no auth locally)
	Collection string // collection name, sanitized
	HTTP       *http.Client
}

// NewQdrant validates and normalizes config; errors on an empty/unsafe base or
// collection so callers fall back to local rather than build bad requests.
func NewQdrant(base, apiKey, collection string) (*Qdrant, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return nil, fmt.Errorf("vectordb: qdrant base url empty")
	}
	if !(strings.HasPrefix(base, "http://") || strings.HasPrefix(base, "https://")) {
		return nil, fmt.Errorf("vectordb: qdrant base url must be http(s)")
	}
	col := sanitizeCollection(collection)
	if col == "" {
		col = "reponest_vecs"
	}
	return &Qdrant{Base: base, APIKey: apiKey, Collection: col, HTTP: &http.Client{Timeout: 20 * time.Second}}, nil
}

func (q *Qdrant) Name() string { return "qdrant" }

// Reachable is a cheap connectivity probe (list collections).
func (q *Qdrant) Reachable() bool {
	_, err := q.do(http.MethodGet, "/collections", nil, nil)
	return err == nil
}

// Ensure creates the collection at `dim` if absent (an "already exists" reply is OK).
func (q *Qdrant) Ensure(dim int) error {
	if dim < 1 {
		return fmt.Errorf("vectordb: qdrant ensure invalid dim %d", dim)
	}
	body := map[string]any{"vectors": map[string]any{"size": dim, "distance": "Cosine"}}
	resp, err := q.do(http.MethodPut, "/collections/"+q.Collection, body, nil)
	if err != nil {
		return err
	}
	// 409 / "already exists" is expected when the collection is present.
	if resp.StatusCode == http.StatusConflict || strings.Contains(resp.BodyStr, "already exists") {
		return nil
	}
	return statusErr("ensure collection", resp)
}

// Clear drops and recreates the collection (a full re-embed boundary).
func (q *Qdrant) Clear(dim int) error {
	if _, err := q.do(http.MethodDelete, "/collections/"+q.Collection, nil, nil); err != nil {
		return err
	}
	return q.Ensure(dim)
}

// Upsert writes/replaces one point (id = note id, payload carries note_id too).
func (q *Qdrant) Upsert(id int64, vec []float32) error {
	point := map[string]any{"id": id, "vector": vec, "payload": map[string]any{"note_id": id}}
	body := map[string]any{"points": []any{point}, "wait": true}
	resp, err := q.do(http.MethodPut, "/collections/"+q.Collection+"/points?wait=true", body, nil)
	if err != nil {
		return err
	}
	return statusErr("upsert", resp)
}

// Delete removes one point by id. Qdrant answers 2xx for a point that never
// existed, so this is idempotent without any extra handling.
func (q *Qdrant) Delete(id int64) error {
	body := map[string]any{"points": []any{id}, "wait": true}
	resp, err := q.do(http.MethodDelete, "/collections/"+q.Collection+"/points?wait=true", body, nil)
	if err != nil {
		return err
	}
	return statusErr("delete", resp)
}

type qdrantSearchResp struct {
	Result []struct {
		ID int64 `json:"id"`
	} `json:"result"`
}

// Search returns the note ids of the most similar points, nearest first.
func (q *Qdrant) Search(vec []float32, limit int) ([]int64, error) {
	if limit <= 0 {
		return nil, nil
	}
	body := map[string]any{"vector": vec, "limit": limit}
	var out qdrantSearchResp
	resp, err := q.do(http.MethodPost, "/collections/"+q.Collection+"/points/search", body, &out)
	if err != nil {
		return nil, err
	}
	if err := statusErr("search", resp); err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(out.Result))
	for _, r := range out.Result {
		ids = append(ids, r.ID)
	}
	return ids, nil
}

type httpResp struct {
	StatusCode int
	BodyStr    string
}

func (q *Qdrant) do(method, path string, body any, out any) (httpResp, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return httpResp{}, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, q.Base+path, rdr)
	if err != nil {
		return httpResp{}, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if q.APIKey != "" {
		req.Header.Set("api-key", q.APIKey)
	}
	resp, err := q.HTTP.Do(req)
	if err != nil {
		return httpResp{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	hr := httpResp{StatusCode: resp.StatusCode, BodyStr: string(raw)}
	if err == nil && out != nil && resp.StatusCode == http.StatusOK {
		if e := json.Unmarshal(raw, out); e != nil {
			return hr, fmt.Errorf("vectordb: qdrant decode: %w", e)
		}
	}
	return hr, nil
}

func statusErr(op string, r httpResp) error {
	if r.StatusCode >= 200 && r.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("vectordb: qdrant %s: %d %s", op, r.StatusCode, firstLine(r.BodyStr))
}

// sanitizeCollection keeps only [A-Za-z0-9_-], preventing path/host injection
// from a user-supplied collection name.
func sanitizeCollection(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if r == '-' || r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	if len(s) > 160 {
		return s[:160]
	}
	return s
}
