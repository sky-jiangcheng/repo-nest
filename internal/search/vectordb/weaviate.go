package vectordb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Weaviate is a remote/self-hosted Weaviate server (REST v1) — the second
// "axis B" vector-store backend after Qdrant (ADR-0013). Objects carry a
// note_id int property + the vector; search uses the nearVector GraphQL query,
// which returns objects ordered by ascending distance.
//
// Verified against a live Weaviate container: create {class,vectorizer:none,
// properties:[note_id int]}; POST /v1/objects (409/422 on dup id → PUT to upsert);
// POST /v1/graphql Get{ <Class>(nearVector:{vector:[...]}){note_id ...} }.
// Weaviate capitalises the class name (repo_smoke -> Repo_smoke), so we
// normalise to a capitalised identifier and reuse it for create and query.
type Weaviate struct {
	Base   string // e.g. http://localhost:8080
	APIKey string // optional (X-Weaviate-Api-Key)
	Class  string // normalised capitalised class name
	HTTP   *http.Client
}

// NewWeaviant validates/normalises config; errors on non-http base so Open()
// falls back to local rather than build bad requests.
func NewWeaviate(base, apiKey, collection string) (*Weaviate, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if !(strings.HasPrefix(base, "http://") || strings.HasPrefix(base, "https://")) {
		return nil, fmt.Errorf("vectordb: weaviate base url must be http(s)")
	}
	class := normaliseClass(collection)
	if class == "" {
		class = "RepoNestVec"
	}
	return &Weaviate{Base: base, APIKey: apiKey, Class: class, HTTP: &http.Client{Timeout: 20 * time.Second}}, nil
}

func (w *Weaviate) Name() string { return "weaviate" }

// Reachable probes the readiness endpoint.
func (w *Weaviate) Reachable() bool {
	_, code, err := w.do(http.MethodGet, "/v1/.well-known/ready", nil, nil)
	return err == nil && code == http.StatusOK
}

// Ensure creates the class (with the note_id int property) if absent.
func (w *Weaviate) Ensure(dim int) error {
	if dim < 1 {
		return fmt.Errorf("vectordb: weaviate ensure invalid dim %d", dim)
	}
	if _, code, err := w.do(http.MethodGet, "/v1/schema/"+w.Class, nil, nil); err == nil && code == http.StatusOK {
		return nil // exists
	}
	body := map[string]any{
		"class":      w.Class,
		"vectorizer": "none",
		"properties": []any{map[string]any{"name": "note_id", "dataType": []string{"int"}}},
	}
	_, code, err := w.do(http.MethodPost, "/v1/schema", body, nil)
	if err != nil {
		return err
	}
	if code >= 400 {
		return fmt.Errorf("vectordb: weaviate create class: %d", code)
	}
	return nil
}

// Clear drops and recreates the class (full re-embed boundary).
func (w *Weaviate) Clear(dim int) error {
	if _, _, err := w.do(http.MethodDelete, "/v1/schema/"+w.Class, nil, nil); err != nil {
		return err
	}
	return w.Ensure(dim)
}

// Upsert writes/replaces one note's vector at a deterministic uuid derived from
// its id. POST creates; on a conflict we PUT to upsert (rebuild Clear()s first,
// so the create path is the common one).
func (w *Weaviate) Upsert(id int64, vec []float32) error {
	body := map[string]any{
		"class":      w.Class,
		"id":         uuidFor(id),
		"vector":     vec,
		"properties": map[string]any{"note_id": id},
	}
	_, code, err := w.do(http.MethodPost, "/v1/objects", body, nil)
	if err == nil && code >= 200 && code < 300 {
		return nil
	}
	_, code, err = w.do(http.MethodPut, "/v1/objects/"+w.Class+"/"+uuidFor(id), body, nil)
	if err != nil {
		return err
	}
	if code >= 400 {
		return fmt.Errorf("vectordb: weaviate upsert note %d: %d", id, code)
	}
	return nil
}

// Delete removes one object by the same deterministic uuid Upsert used. A 404
// means the object was never written or is already gone — which is exactly the
// state the caller asked for, so it is not an error (the interface requires
// idempotency; a note can be deleted before it was ever embedded).
func (w *Weaviate) Delete(id int64) error {
	_, code, err := w.do(http.MethodDelete, "/v1/objects/"+w.Class+"/"+uuidFor(id), nil, nil)
	if err != nil {
		return err
	}
	if code == http.StatusNotFound {
		return nil
	}
	if code >= 400 {
		return fmt.Errorf("vectordb: weaviate delete note %d: %d", id, code)
	}
	return nil
}

type weavSearchResp struct {
	Data struct {
		Get map[string][]struct {
			NoteID int64 `json:"note_id"`
		} `json:"Get"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// Search returns the note ids nearest to vec, most-similar first.
func (w *Weaviate) Search(vec []float32, limit int) ([]int64, error) {
	if limit <= 0 {
		return nil, nil
	}
	q := fmt.Sprintf(`{ Get { %s(nearVector:{vector:[%s]} limit:%d){ note_id _additional{distance} } } }`,
		w.Class, vectorLiteral(vec), limit)
	var out weavSearchResp
	_, code, err := w.do(http.MethodPost, "/v1/graphql", map[string]string{"query": q}, &out)
	if err != nil {
		return nil, err
	}
	if code >= 400 {
		return nil, fmt.Errorf("vectordb: weaviate search: %d", code)
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("vectordb: weaviate graphql: %s", out.Errors[0].Message)
	}
	for _, rows := range out.Data.Get { // single class key
		ids := make([]int64, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.NoteID)
		}
		return ids, nil
	}
	return nil, nil
}

func (w *Weaviate) do(method, path string, body any, out any) ([]byte, int, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, w.Base+path, rdr)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if w.APIKey != "" {
		req.Header.Set("X-Weaviate-Api-Key", w.APIKey)
	}
	resp, err := w.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err == nil && out != nil && resp.StatusCode == http.StatusOK {
		if e := json.Unmarshal(raw, out); e != nil {
			return raw, resp.StatusCode, fmt.Errorf("vectordb: weaviate decode: %w", e)
		}
	}
	return raw, resp.StatusCode, nil
}

// uuidFor maps a positive note id to a stable UUID (48 bits, plenty for note
// ids) so a rebuild re-uses the same object (idempotent) and search can match.
func uuidFor(id int64) string {
	return fmt.Sprintf("00000000-0000-0000-0000-%012x", uint64(id))
}

// vectorLiteral renders floats as a GraphQL array body e.g. "1,0.5,0".
func vectorLiteral(vec []float32) string {
	parts := make([]string, len(vec))
	for i, f := range vec {
		parts[i] = strconv.FormatFloat(float64(f), 'g', -1, 32)
	}
	return strings.Join(parts, ",")
}

// normaliseClass keeps Weaviate-legal identifier chars and capitalises the
// first rune (Weaviate does the same and GraphQL needs the stored name).
func normaliseClass(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		case r == '-':
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return ""
	}
	return strings.ToUpper(out[:1]) + out[1:]
}
