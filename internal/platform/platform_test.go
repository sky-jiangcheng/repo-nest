package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestMain redirects every user-directory lookup into a throwaway tree before
// any test runs. The real implementations create directories (MkdirAll) and
// fire the once-per-process legacy reponest->repotory rename, and these tests
// must never touch the developer's actual HOME — before the isolation the
// suite created ~/Library/Application Support/repotory for real and could
// rename a genuine legacy data directory.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "platform-test-home")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(home)
	os.Setenv("HOME", home)
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	os.Setenv("XDG_STATE_HOME", filepath.Join(home, ".state"))
	if runtime.GOOS == "windows" {
		os.Setenv("AppData", filepath.Join(home, "AppData", "Roaming"))
	}
	os.Exit(m.Run())
}

func TestDefaultScanRoots(t *testing.T) {
	roots := DefaultScanRoots()
	if len(roots) == 0 {
		t.Fatal("DefaultScanRoots returned empty slice")
	}
	for _, root := range roots {
		if root == "" {
			t.Error("DefaultScanRoots contains empty path")
		}
	}
}

func TestGetWindowsDrives(t *testing.T) {
	drives := getWindowsDrives()
	if runtime.GOOS != "windows" && len(drives) != 0 {
		t.Error("getWindowsDrives should return nil on non-Windows")
	}
}

func TestGetGitUserName(t *testing.T) {
	name := GetGitUserName()
	if name == "" {
		t.Error("GetGitUserName returned empty string")
	}
	if name == "unknown" {
		t.Log("Git user name not configured, using fallback")
	}
}

func TestGetDbPath(t *testing.T) {
	path := GetDbPath()
	if path == "" {
		t.Fatal("GetDbPath returned empty string")
	}
	if !strings.HasSuffix(path, filepath.Join("repotory", "repotory.db")) {
		t.Errorf("GetDbPath = %s, want it inside a repotory directory ending with repotory.db", path)
	}
}

func TestGetLogPath(t *testing.T) {
	path := GetLogPath()
	if path == "" {
		t.Fatal("GetLogPath returned empty string")
	}
	if !strings.HasSuffix(path, "repotory.log") {
		t.Errorf("GetLogPath = %s, want it to end with repotory.log", path)
	}
}

func TestMigrateLegacyDataRenamesDir(t *testing.T) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Skipf("no user config dir in test env: %v", err)
	}
	legacy := filepath.Join(configDir, legacyDirName)
	current := filepath.Join(configDir, dirName)
	t.Cleanup(func() { _ = os.RemoveAll(legacy); _ = os.RemoveAll(current) })
	_ = os.RemoveAll(legacy)
	_ = os.RemoveAll(current)
	if err := os.MkdirAll(filepath.Join(legacy, "plugins"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "dashboard.db"), []byte("legacy-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Direct call: idempotent and independent of the package-level sync.Once,
	// so the test exercises the real migration regardless of execution order.
	migrateLegacyData()
	if _, err := os.Stat(filepath.Join(current, "dashboard.db")); err != nil {
		t.Fatalf("legacy contents not carried over: %v", err)
	}
	if _, err := os.Stat(filepath.Join(current, "plugins")); err != nil {
		t.Fatalf("plugins subtree not carried over: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Error("legacy directory still present after rename")
	}
}

func TestMigrateLegacyDataNoopWhenCurrentExists(t *testing.T) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Skipf("no user config dir in test env: %v", err)
	}
	legacy := filepath.Join(configDir, legacyDirName)
	current := filepath.Join(configDir, dirName)
	t.Cleanup(func() { _ = os.RemoveAll(legacy); _ = os.RemoveAll(current) })
	_ = os.RemoveAll(legacy)
	_ = os.RemoveAll(current)
	if err := os.MkdirAll(legacy, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(current, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(current, "marker"), []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	migrateLegacyData()
	if _, err := os.Stat(legacy); err != nil {
		t.Error("legacy directory should be left untouched when the current one exists")
	}
	if _, err := os.Stat(filepath.Join(current, "marker")); err != nil {
		t.Error("current directory must never be replaced by the legacy one")
	}
}

func TestMigrateLegacyLog(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, legacyDirName+".log"), []byte("log"), 0o600); err != nil {
		t.Fatal(err)
	}
	migrateLegacyLog(dir)
	if _, err := os.Stat(filepath.Join(dir, dirName+".log")); err != nil {
		t.Fatalf("log file not renamed: %v", err)
	}
}

func TestMigrateLegacyDbFile(t *testing.T) {
	t.Run("renames the legacy db and its sidecars", func(t *testing.T) {
		dir := t.TempDir()
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if err := os.WriteFile(filepath.Join(dir, "dashboard.db"+suffix), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		migrateLegacyDbFile(dir)
		for _, suffix := range []string{"", "-wal", "-shm"} {
			renamed := filepath.Join(dir, "repotory.db"+suffix)
			if _, err := os.Stat(renamed); err != nil {
				t.Errorf("repotory.db%s not in place: %v", suffix, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "dashboard.db"+suffix)); !os.IsNotExist(err) {
				t.Errorf("dashboard.db%s still present after rename", suffix)
			}
		}
	})

	t.Run("an existing repotory.db always wins", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "dashboard.db"), []byte("legacy"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "repotory.db"), []byte("current"), 0o600); err != nil {
			t.Fatal(err)
		}
		migrateLegacyDbFile(dir)
		got, err := os.ReadFile(filepath.Join(dir, "repotory.db"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "current" {
			t.Fatalf("repotory.db was overwritten by the legacy file: %q", got)
		}
		if _, err := os.Stat(filepath.Join(dir, "dashboard.db")); err != nil {
			t.Fatalf("legacy dashboard.db should be left in place when the new name exists: %v", err)
		}
	})

	t.Run("no legacy file is a no-op", func(t *testing.T) {
		dir := t.TempDir()
		migrateLegacyDbFile(dir) // must not create anything or panic
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Fatalf("unexpected files created: %v", entries)
		}
	})
}
