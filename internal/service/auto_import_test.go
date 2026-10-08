package service

import (
	"testing"

	"repo-nest/internal/db"
)

// The predicate behind "does the app read other tools' memory at startup".
//
// The regression this pins is the ON-by-absence bug: db.GetConfig maps
// sql.ErrNoRows to ("", nil), so the old `err == nil && v != "0"` test returned
// true for a row that did not exist. A setting documented as "the user can
// disable it" was therefore enabled for precisely the people who never enabled
// it — and no UI action could produce that state on an existing database either,
// because the row was always seeded. "1" is the only value that means yes.
func TestAutoImportEnabledIsExplicitOptIn(t *testing.T) {
	svc, _ := setupService(t)

	cases := []struct {
		name  string
		set   func() error
		want  bool
		notes string
	}{
		{"fresh seeded value", func() error { return nil }, false,
			"a new database ships OFF"},
		{"explicit 1", func() error { return db.SetConfig(svc.db, "auto_import", "1") }, true,
			"the only enabling value"},
		{"explicit 0", func() error { return db.SetConfig(svc.db, "auto_import", "0") }, false,
			"the documented disable value"},
		{"junk value", func() error { return db.SetConfig(svc.db, "auto_import", "yes") }, false,
			"anything unrecognized is OFF, never ON"},
		{"empty string", func() error { return db.SetConfig(svc.db, "auto_import", "") }, false,
			"GetConfig returns \"\" for both empty and absent — both must be OFF"},
		{"row deleted", func() error {
			_, err := svc.db.Exec("DELETE FROM app_config WHERE key = 'auto_import'")
			return err
		}, false, "the bug case: an absent row used to mean ON"},
	}

	for _, tc := range cases {
		if err := tc.set(); err != nil {
			t.Fatalf("%s: setup: %v", tc.name, err)
		}
		if got := svc.autoImportEnabled(); got != tc.want {
			t.Errorf("%s: autoImportEnabled() = %v, want %v (%s)", tc.name, got, tc.want, tc.notes)
		}
	}
}
