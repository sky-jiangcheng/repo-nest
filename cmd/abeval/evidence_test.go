package main

import (
	"path/filepath"
	"strings"
	"testing"

	"repotory/internal/db"
	"repotory/internal/search/abeval"
)

func TestLoadEvidenceCasesTyped(t *testing.T) {
	p := writeCases(t, `{"query":"q1","relevant":[{"type":"page","id":5},{"type":"note","id":7}]}
{"query":"q2","relevant":[{"type":"page","id":6}]}

# a comment line
{"query":"q3","relevant":[]}
`)
	cases, err := loadEvidenceCases(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 3 {
		t.Fatalf("loaded %d cases, want 3 (blank + comment lines skipped)", len(cases))
	}
	if cases[0].Query != "q1" {
		t.Errorf("case 0 query = %q", cases[0].Query)
	}
	want := []abeval.DocID{{Type: "page", ID: 5}, {Type: "note", ID: 7}}
	if len(cases[0].Relevant) != len(want) {
		t.Fatalf("case 0 relevant = %+v", cases[0].Relevant)
	}
	for i, d := range want {
		if cases[0].Relevant[i] != d {
			t.Errorf("case 0 relevant[%d] = %+v, want %+v", i, cases[0].Relevant[i], d)
		}
	}
}

// A bare number is the notes format. Accepting it here would let a notes-labeled
// file load against the evidence gate, where the same number means a page —
// exactly the cross-type collision DocID exists to prevent. The loader must
// refuse it rather than quietly score against the wrong id space.
//
// The refusal comes from json.Unmarshal (a number cannot decode into DocID),
// which is the right layer: the type system already rejects the ambiguity, so
// the loader needs no bespoke branch to do it.
func TestLoadEvidenceCasesRejectsBareIDs(t *testing.T) {
	p := writeCases(t, `{"query":"q1","relevant":[5]}`+"\n")
	_, err := loadEvidenceCases(p)
	if err == nil {
		t.Fatal("bare int64 relevant was accepted; a notes-format file must not load here")
	}
	if !strings.Contains(err.Error(), "DocID") {
		t.Errorf("error should name the type it could not decode into, got: %v", err)
	}
}

func TestLoadEvidenceCasesRejectsUnknownType(t *testing.T) {
	p := writeCases(t, `{"query":"q1","relevant":[{"type":"todo","id":5}]}`+"\n")
	if _, err := loadEvidenceCases(p); err == nil {
		t.Fatal("an unknown doc type was accepted; only page|note exist")
	}
}

func TestLoadEvidenceCasesRejectsEmptyQuery(t *testing.T) {
	p := writeCases(t, `{"query":"","relevant":[{"type":"page","id":1}]}`+"\n")
	if _, err := loadEvidenceCases(p); err == nil {
		t.Fatal("an empty query was accepted; it would score as an unmatched case")
	}
}

// scalarInt backs the density diagnostic. A broken query must degrade to -1, not
// abort: the diagnostic is context for the verdict and must never change it.
func TestScalarIntReportsFailureAsMinusOne(t *testing.T) {
	database, err := db.InitDB(filepath.Join(t.TempDir(), "diag.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()

	if got := scalarInt(database, "SELECT count(*) FROM no_such_table"); got != -1 {
		t.Errorf("scalarInt on a bad query = %d, want -1", got)
	}
	if got := scalarInt(database, "SELECT count(*) FROM page_links"); got != 0 {
		t.Errorf("scalarInt on an empty table = %d, want 0", got)
	}
}
