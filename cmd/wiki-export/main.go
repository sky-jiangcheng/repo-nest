// Command wiki-export renders RepoNest's derived wiki layer into a Markdown tree
// (ADR-0014 决策 2: one-way export; TODO M6-W1b).
//
// It is its own binary because the desktop `reponest` executable has no
// subcommand dispatch at all — it launches Wails and returns — so the "reponest
// wiki export" phrasing in the ADR would be a lie. Same pattern as cmd/vector-init
// and cmd/abeval.
//
//	go build -o reponest-wiki-export ./cmd/wiki-export
//	./reponest-wiki-export -out ./wiki-out            # refuses a non-empty target
//	./reponest-wiki-export -out ./wiki-out -force     # overwrite our own subtree
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"repo-nest/internal/db"
	"repo-nest/internal/platform"
	"repo-nest/internal/service"
	"repo-nest/internal/version"
)

func main() {
	log.SetPrefix("wiki-export ")
	out := flag.String("out", "", "output directory (a wiki/ subtree is created inside it)")
	dbPath := flag.String("db", "", "database path (default: the app's DB)")
	projectID := flag.Int64("project", 0, "restrict the export to one project id (0 = all)")
	force := flag.Bool("force", false, "overwrite an existing target directory")
	flag.Parse()

	if *out == "" {
		fmt.Fprintln(os.Stderr, "wiki-export: --out is required")
		flag.Usage()
		os.Exit(2)
	}

	path := *dbPath
	if path == "" {
		path = platform.GetDbPath()
	}
	database, err := db.InitDB(path)
	if err != nil {
		log.Fatalf("open database %s: %v", path, err)
	}
	defer func() { _ = database.Close() }()

	report, err := service.New(database, "").ExportWikiTree(*out, *projectID, *force)
	if err != nil {
		log.Fatalf("export failed: %v", err)
	}

	fmt.Printf("RepoNest %s — exported %d page(s) to %s\n", version.Version, report.Pages, report.Root)
	fmt.Printf("  files=%d skipped_pages=%d dangling_links=%d\n",
		len(report.Files), report.SkippedPages, report.DanglingLinks)
	for _, f := range report.Files {
		fmt.Printf("  %-52s %6d B  %s\n", f.Path, f.Bytes, f.SHA256[:12])
	}
	if report.DanglingLinks > 0 {
		fmt.Println("  note: some [[wikilinks]] in page bodies point at pages that do not exist")
	}
	if len(report.StaleFiles) > 0 {
		fmt.Printf("  stale files under wiki/ (written by an earlier export, not by this one — remove them if you want a clean tree):\n")
		for _, f := range report.StaleFiles {
			fmt.Printf("    %s\n", f)
		}
	}
}
