package knowledge

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// maxManifestReadBytes caps dependency-manifest reads. These files live in
// repositories the user chose to scan, so they are not attacker-controlled,
// but nothing stops a scanned repo from containing a pathological file (a
// vendored/generated package.json can reach hundreds of megabytes). Only
// dependency names are extracted, so a megabyte is already generous; reading
// beyond it is pure memory risk with zero information gain.
const maxManifestReadBytes = 1 << 20

// readCapped reads at most limit bytes from path. A truncated manifest fails
// its parser and is skipped by the caller (no deps from that file) instead of
// being loaded whole into memory.
func readCapped(path string, limit int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, int64(limit)))
}

// ErrNotARepo is returned when the path is not an accessible directory.
var ErrNotARepo = errors.New("not an accessible repository directory")

// Mine aggregates README, tech stack, language breakdown, dependencies,
// top contributors and recent activity for a repository.
func Mine(repoPath string) (*RepoKnowledge, error) {
	readme, err := ExtractREADME(repoPath)
	if err != nil && err != ErrNotARepo {
		// A non-repo path yields empty knowledge, not a hard failure for callers.
		readme = ""
	}

	techs, err := DetectTechStack(repoPath)
	if err != nil {
		techs = nil
	}
	langs, err := DetectLanguages(repoPath)
	if err != nil {
		langs = nil
	}

	k := &RepoKnowledge{
		ReadmeExcerpt: readme,
		TechStack:     []Tech{},
		Languages:     []LanguageStat{},
	}
	if len(techs) > 0 {
		k.TechStack = techs
	}
	if len(langs) > 0 {
		k.Languages = langs
	}

	deps, err := DetectDependencies(repoPath)
	if err == nil {
		k.Dependencies = deps
	}

	contribs, err := DetectContributors(repoPath, 5)
	if err == nil {
		k.TopContributors = contribs
	}

	activity, err := DetectActivity(repoPath)
	if err == nil {
		k.Activity = activity
	}

	return k, nil
}

// isReadme reports whether a filename is a README (case-insensitive, any extension).
func isReadme(name string) bool {
	base := strings.ToLower(name)
	if !strings.HasPrefix(base, "readme") {
		return false
	}
	return base == "readme" || strings.HasPrefix(base, "readme.")
}

// maxReadmeBytes bounds how much of a README we keep (enough to preview).
const maxReadmeBytes = 8 * 1024

// maxReadmeLines bounds the number of lines kept.
const maxReadmeLines = 200

// ExtractREADME finds and reads the repository's README, returning a bounded
// excerpt. Returns an empty string (no error) when no README is present.
func ExtractREADME(repoPath string) (string, error) {
	info, err := os.Stat(repoPath)
	if err != nil || !info.IsDir() {
		return "", ErrNotARepo
	}

	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return "", nil
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !isReadme(name) {
			continue
		}
		f, err := os.Open(filepath.Join(repoPath, name))
		if err != nil {
			continue
		}
		defer f.Close()

		var sb strings.Builder
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		lineNo := 0
		for scanner.Scan() {
			line := scanner.Text()
			sb.WriteString(line)
			sb.WriteByte('\n')
			lineNo++
			if sb.Len() >= maxReadmeBytes || lineNo >= maxReadmeLines {
				break
			}
		}
		return strings.TrimSpace(sb.String()), nil
	}
	return "", nil
}

// techManifest maps a top-level manifest filename to the tech it implies.
var techManifest = map[string]Tech{
	"package.json":       {"JavaScript / TypeScript", "language"},
	"go.mod":             {"Go", "language"},
	"Cargo.toml":         {"Rust", "language"},
	"pom.xml":            {"Java (Maven)", "language"},
	"build.gradle":       {"Java (Gradle)", "language"},
	"build.gradle.kts":   {"Java (Gradle)", "language"},
	"requirements.txt":   {"Python", "language"},
	"pyproject.toml":     {"Python", "language"},
	"setup.py":           {"Python", "language"},
	"composer.json":      {"PHP", "language"},
	"Gemfile":            {"Ruby", "language"},
	"Package.swift":      {"Swift", "language"},
	"pubspec.yaml":       {"Dart / Flutter", "framework"},
	"mix.exs":            {"Elixir", "language"},
	"CMakeLists.txt":     {"C / C++", "language"},
	"docker-compose.yml": {"Docker Compose", "tool"},
	"Dockerfile":         {"Docker", "tool"},
}

// maxScanFiles bounds the language-counting walk so huge monorepos stay fast.
const maxScanFiles = 20000

// maxLanguageFileBytes skips files larger than this during line counting. A
// multi-megabyte data file (vendored JSON dataset, minified bundle, generated
// lockfile with a countable extension) would otherwise be read line-by-line
// in full — the walk is bounded by file COUNT, not size. Skipping it barely
// moves the language stats and keeps mining fast.
const maxLanguageFileBytes = 1 << 20 // 1 MiB

// skipDirs are directory names we never descend into when counting languages.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true,
	"build": true, "target": true, ".venv": true, "venv": true, "__pycache__": true,
	".idea": true, ".vscode": true, "Pods": true, ".next": true, ".cache": true,
}

// extLanguage maps a file extension to a language label.
var extLanguage = map[string]string{
	".go": "Go", ".js": "JavaScript", ".jsx": "JavaScript", ".mjs": "JavaScript",
	".ts": "TypeScript", ".tsx": "TypeScript",
	".py": "Python", ".rb": "Ruby", ".rs": "Rust",
	".java": "Java", ".kt": "Kotlin", ".scala": "Scala",
	".c": "C", ".h": "C", ".cpp": "C++", ".cc": "C++", ".hpp": "C++",
	".cs": "C#", ".php": "PHP", ".swift": "Swift",
	".m": "Objective-C", ".mm": "Objective-C++",
	".vue": "Vue", ".svelte": "Svelte",
	".sh": "Shell", ".bash": "Shell", ".zsh": "Shell",
	".lua": "Lua", ".ex": "Elixir", ".exs": "Elixir",
	".clj": "Clojure", ".dart": "Dart",
	".sql": "SQL", ".html": "HTML", ".css": "CSS", ".scss": "SCSS",
	".json": "JSON", ".yaml": "YAML", ".yml": "YAML", ".toml": "TOML",
	".md": "Markdown",
}

// DetectTechStack inspects top-level manifest files and returns detected tech.
func DetectTechStack(repoPath string) ([]Tech, error) {
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return nil, ErrNotARepo
	}

	var techs []Tech
	seen := make(map[string]bool)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if t, ok := techManifest[name]; ok {
			key := t.Name
			if !seen[key] {
				seen[key] = true
				techs = append(techs, t)
			}
		}
	}

	// C# projects: any *.csproj at top level.
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(e.Name()), ".csproj") {
			if !seen["C#"] {
				seen["C#"] = true
				techs = append(techs, Tech{Name: "C#", Category: "language"})
			}
		}
	}

	sort.Slice(techs, func(i, j int) bool { return techs[i].Name < techs[j].Name })
	return techs, nil
}

// DetectLanguages walks the repo counting lines per language extension, skipping
// dependency/build directories. Returns the top languages by line count.
func DetectLanguages(repoPath string) ([]LanguageStat, error) {
	counts := make(map[string]int)
	scanned := 0

	err := filepath.WalkDir(repoPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if lang, ok := extLanguage[ext]; ok {
			if info, ierr := d.Info(); ierr == nil && info.Size() > maxLanguageFileBytes {
				return nil
			}
			if f, ferr := os.Open(path); ferr == nil {
				lineCount := 0
				scanner := bufio.NewScanner(f)
				scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
				for scanner.Scan() {
					lineCount++
				}
				f.Close()
				// A read error (I/O failure, ErrTooLong) makes the count a
				// partial number; adding it would misstate the tree.
				if scanner.Err() == nil {
					counts[lang] += lineCount
				}
			}
		}
		scanned++
		if scanned > maxScanFiles {
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	stats := make([]LanguageStat, 0, len(counts))
	for lang, n := range counts {
		stats = append(stats, LanguageStat{Language: lang, Count: n})
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Count != stats[j].Count {
			return stats[i].Count > stats[j].Count
		}
		return stats[i].Language < stats[j].Language
	})

	if len(stats) > 8 {
		stats = stats[:8]
	}
	return stats, nil
}

// DetectDependencies detects npm / go / cargo dependencies from manifest files.
func DetectDependencies(repoPath string) ([]Dependency, error) {
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return nil, nil
	}
	var deps []Dependency
	seen := make(map[string]bool)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		switch name {
		case "package.json":
			d, err := parseNpmDeps(repoPath, name)
			if err == nil {
				deps = append(deps, d...)
				for _, dep := range d {
					seen[dep.Name] = true
				}
			}
		case "go.mod":
			d, err := parseGoDeps(repoPath)
			if err == nil {
				for _, dep := range d {
					if !seen[dep.Name] {
						deps = append(deps, dep)
						seen[dep.Name] = true
					}
				}
			}
		case "Cargo.toml":
			d, err := parseCargoDeps(repoPath)
			if err == nil {
				for _, dep := range d {
					if !seen[dep.Name] {
						deps = append(deps, dep)
						seen[dep.Name] = true
					}
				}
			}
		}
	}
	if len(deps) > 30 {
		deps = deps[:30]
	}
	sort.Slice(deps, func(i, j int) bool { return deps[i].Name < deps[j].Name })
	return deps, nil
}

func parseNpmDeps(repoPath, filename string) ([]Dependency, error) {
	data, err := readCapped(filepath.Join(repoPath, filename), maxManifestReadBytes)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Dependencies map[string]string `json:"dependencies"`
		DevDeps      map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	var deps []Dependency
	for name, ver := range raw.Dependencies {
		deps = append(deps, Dependency{Name: name, Version: ver, Source: "npm"})
	}
	for name, ver := range raw.DevDeps {
		deps = append(deps, Dependency{Name: name, Version: ver, Source: "npm"})
	}
	return deps, nil
}

func parseGoDeps(repoPath string) ([]Dependency, error) {
	data, err := readCapped(filepath.Join(repoPath, "go.mod"), maxManifestReadBytes)
	if err != nil {
		return nil, err
	}
	var deps []Dependency
	inBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "require ("):
			inBlock = true
			continue
		case inBlock && line == ")":
			inBlock = false
			continue
		}
		var fields []string
		if inBlock {
			fields = strings.Fields(line)
		} else if strings.HasPrefix(line, "require ") {
			fields = strings.Fields(strings.TrimPrefix(line, "require "))
		} else {
			continue
		}
		if len(fields) < 2 || strings.HasPrefix(fields[0], "//") {
			continue
		}
		if fields[0] == "replace" || fields[0] == "exclude" {
			continue
		}
		deps = append(deps, Dependency{Name: fields[0], Version: fields[1], Source: "go"})
	}
	return deps, nil
}

func parseCargoDeps(repoPath string) ([]Dependency, error) {
	data, err := readCapped(filepath.Join(repoPath, "Cargo.toml"), maxManifestReadBytes)
	if err != nil {
		return nil, err
	}
	var deps []Dependency
	inSection := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inSection = line
			continue
		}
		if (inSection == "[dependencies]" || inSection == "[dev-dependencies]" || inSection == "[build-dependencies]") && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			name := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			version := ""
			var inner struct{ Version string }
			if err := json.Unmarshal([]byte(val), &inner); err == nil && inner.Version != "" {
				version = inner.Version
			} else {
				version = strings.Trim(val, `"`)
			}
			if name != "" && version != "" {
				src := "cargo"
				if inSection == "[dev-dependencies]" || inSection == "[build-dependencies]" {
					src = "cargo (dev)"
				}
				deps = append(deps, Dependency{Name: name, Version: version, Source: src})
			}
		}
	}
	return deps, nil
}

// DetectContributors returns the top N contributors by commit count.
func DetectContributors(repoPath string, limit int) ([]TopContributor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// `HEAD` is required: with no rev argument `git shortlog` reads its log
	// from stdin, and with a nil (non-TTY) stdin that is /dev/null — always
	// empty output, so the contributor list was silently empty forever. The
	// old `-n<limit>` flag was also wrong: shortlog forwards it to rev-list,
	// where it means --max-count (only the last N commits are scanned), not
	// "top N authors". Truncation to `limit` happens in Go after the count.
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "shortlog", "-sn", "--no-merges", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return nil, nil
	}
	var contributors []TopContributor
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 2 {
			continue
		}
		count, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
		author := strings.TrimSpace(parts[1])
		if count > 0 && author != "" {
			contributors = append(contributors, TopContributor{Author: author, Count: count})
		}
	}
	sort.Slice(contributors, func(i, j int) bool { return contributors[i].Count > contributors[j].Count })
	if limit > 0 && len(contributors) > limit {
		contributors = contributors[:limit]
	}
	return contributors, nil
}

// DetectActivity computes recent commit activity metrics for a repository.
func DetectActivity(repoPath string) (*ActivityStat, error) {
	now := time.Now()
	threeMonthsAgo := now.AddDate(0, -3, 0).Format("2006-01-02")
	monthAgo := now.AddDate(0, -1, 0).Format("2006-01-02")
	today := now.Format("2006-01-02")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	totalCmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-list", "--count", "HEAD")
	totalOut, err := totalCmd.Output()
	totalCommits := 0
	if err == nil {
		totalCommits, _ = strconv.Atoi(strings.TrimSpace(string(totalOut)))
	}

	daysCmd := exec.CommandContext(ctx, "git", "-C", repoPath, "log", "--format=%ad", "--date=short", threeMonthsAgo+".."+today)
	daysOut, err2 := daysCmd.Output()
	activeDays := 0
	if err2 == nil {
		daySet := make(map[string]bool)
		for _, d := range strings.Split(string(daysOut), "\n") {
			d = strings.TrimSpace(d)
			if d != "" {
				daySet[d] = true
			}
		}
		activeDays = len(daySet)
	}

	commitsCmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-list", "--count", monthAgo+"..HEAD")
	commitsOut, err3 := commitsCmd.Output()
	commitRate30d := 0
	if err3 == nil {
		commitRate30d, _ = strconv.Atoi(strings.TrimSpace(string(commitsOut)))
	}

	lastCmd := exec.CommandContext(ctx, "git", "-C", repoPath, "log", "-1", "--format=%ad", "--date=short")
	lastOut, err4 := lastCmd.Output()
	lastDate := ""
	if err4 == nil {
		lastDate = strings.TrimSpace(string(lastOut))
	}

	monthsCmd := exec.CommandContext(ctx, "git", "-C", repoPath, "log", "--format=%ad", "--date=format:%Y-%m", threeMonthsAgo+".."+today)
	monthsOut, err5 := monthsCmd.Output()
	activeMonths := 0
	if err5 == nil {
		monthSet := make(map[string]bool)
		for _, m := range strings.Split(string(monthsOut), "\n") {
			m = strings.TrimSpace(m)
			if m != "" {
				monthSet[m] = true
			}
		}
		activeMonths = len(monthSet)
	}

	return &ActivityStat{
		TotalCommits:   totalCommits,
		ActiveDays:     activeDays,
		LastCommitDate: lastDate,
		CommitRate30d:  commitRate30d,
		ActiveMonths:   activeMonths,
	}, nil
}
