package diff

import (
	"strings"
	"testing"
)

func TestLinesNoChange(t *testing.T) {
	got := Lines("a\nb", "a\nb")
	want := " a\n b"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLinesAdditionAndRemoval(t *testing.T) {
	// The backtrack prefers emitting additions first when both an addition
	// and a removal are possible; this matches the historical behaviour.
	got := Lines("a\nb\nc", "a\nx\nc")
	want := " a\n+x\n-b\n c"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLinesPureAddition(t *testing.T) {
	got := Lines("a\nc", "a\nb\nc")
	want := " a\n+b\n c"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLinesPureRemoval(t *testing.T) {
	got := Lines("a\nb\nc", "a\nc")
	want := " a\n-b\n c"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLinesSameLineCount(t *testing.T) {
	got := Lines("one\ntwo", "one\nTWO")
	want := " one\n+TWO\n-two"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A pathological note (tens of thousands of dissimilar short lines — well
// within the 100 KB content cap) must not ask for a ~gigabyte dp table. The
// diff degrades to "everything removed, everything added" instead of
// fatal-OOMing the process.
func TestLinesHugeInputDegradesInsteadOfOOM(t *testing.T) {
	old := make([]string, 0, 60_000)
	new := make([]string, 0, 60_000)
	for i := 0; i < 60_000; i++ {
		old = append(old, "x")
		new = append(new, "y")
	}
	got := Lines(strings.Join(old, "\n"), strings.Join(new, "\n"))
	lines := strings.Split(got, "\n")
	if len(lines) != 120_000 {
		t.Fatalf("degraded diff should emit all -/+ lines, got %d", len(lines))
	}
	if lines[0][0] != '-' || lines[60_000][0] != '+' {
		t.Fatalf("degraded diff should lead with removals then additions: %q", lines[0])
	}
}

// Common edges are trimmed before the budget check, so a realistic
// one-line edit in a long note still produces the exact historical output
// rather than the degraded form.
func TestLinesSingleEditInLongNoteIsExact(t *testing.T) {
	var old, new []string
	for i := 0; i < 3_000; i++ {
		old = append(old, "line")
		new = append(new, "line")
	}
	old[1_500] = "changed"
	new[1_500] = "CHANGED"
	got := Lines(strings.Join(old, "\n"), strings.Join(new, "\n"))
	// 1500 common lines before the edit, the edited pair (+CHANGED/-changed),
	// and 1499 common lines after it (index 1500 is the edit itself).
	want := strings.Repeat(" line\n", 1500) +
		"+CHANGED\n-changed\n" +
		strings.Repeat(" line\n", 1499)
	want = strings.TrimSuffix(want, "\n")
	if got != want {
		t.Fatalf("exact diff mismatch: got %d lines, want %d", len(strings.Split(got, "\n")), len(strings.Split(want, "\n")))
	}
}
