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
// fire the once-per-process legacy gitbuddy->reponest rename, and these tests
// must never touch the developer's actual HOME — before the isolation the
// suite created ~/Library/Application Support/reponest for real and could
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
	if !strings.HasSuffix(path, filepath.Join("reponest", "dashboard.db")) {
		t.Errorf("GetDbPath = %s, want it inside a reponest directory ending with dashboard.db", path)
	}
}

func TestGetLogPath(t *testing.T) {
	path := GetLogPath()
	if path == "" {
		t.Fatal("GetLogPath returned empty string")
	}
	if !strings.HasSuffix(path, "reponest.log") {
		t.Errorf("GetLogPath = %s, want it to end with reponest.log", path)
	}
}
