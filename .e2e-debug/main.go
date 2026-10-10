// Command .e2e-debug prints, per query, what the FTS arm and the graph arm
// actually return, so an unexpected abeval delta can be traced to a layer.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"repo-nest/internal/db"
	"repo-nest/internal/service"
)

func main() {
	d, err := db.InitDB("/tmp/e2e/real.db")
	if err != nil {
		fmt.Println("init:", err)
		os.Exit(1)
	}
	defer d.Close()
	svc := service.New(d, "debug")

	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer f.Close()
	// Same contract as abeval: a tool that flips a user-database gate key must
	// restore it on exit — pass, fail, or early error.
	original, _ := db.GetConfig(d, "wiki_graph_search")
	restore := func() {
		if original == "" {
			_ = db.DeleteConfig(d, "wiki_graph_search")
		} else {
			_ = db.SetConfig(d, "wiki_graph_search", original)
		}
	}
	defer restore()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		q := line
		if i := strings.Index(line, `"query":"`); i >= 0 {
			rest := line[i+len(`"query":"`):]
			if j := strings.Index(rest, `"`); j >= 0 {
				q = rest[:j]
			}
		}
		_ = db.SetConfig(d, "wiki_graph_search", "0")
		off := svc.EvidencePageRanking(q, 0, 50)
		_ = db.SetConfig(d, "wiki_graph_search", "1")
		on := svc.EvidencePageRanking(q, 0, 50)
		fmt.Printf("Q %-24s off=%v on=%v\n", q, ids(off), ids(on))
	}
}

func ids(items []service.EvidenceItem) []int64 {
	out := make([]int64, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}
