// Package diff shows what would change on the router if `wrtbox apply`
// were run now. It downloads the live bytes of each managed path,
// normalises whitespace the same way the golden test does, and emits
// a per-file minimal unified-style diff.
package diff

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/itslavrov/wrtbox/internal/config"
	"github.com/itslavrov/wrtbox/internal/lists"
	"github.com/itslavrov/wrtbox/internal/render"
	"github.com/itslavrov/wrtbox/internal/ssh"
)

// Options tunes Run.
type Options struct {
	Writer io.Writer
	Lists  lists.Provider
}

// Run downloads the live state and compares to the desired render of
// cfg. It writes per-file diffs to opts.Writer and returns a summary
// (number of changed files, added, removed).
type Summary struct {
	Changed []string
	Added   []string
	Removed []string // paths that exist locally but not on the router (rare)
	Missing []string // paths that exist on the router but not in the render (rare)
}

// Run performs the diff.
func Run(ctx context.Context, exec ssh.Executor, cfg *config.Config, opts Options) (*Summary, error) {
	w := opts.Writer
	if w == nil {
		w = io.Discard
	}
	files, err := render.Render(ctx, cfg, render.Options{Lists: opts.Lists})
	if err != nil {
		return nil, fmt.Errorf("diff: render: %w", err)
	}

	sum := &Summary{}
	for _, f := range files {
		remotePath := "/" + f.Path
		remote, err := exec.Download(ctx, remotePath)
		if err != nil {
			// Missing file on router = full add.
			sum.Added = append(sum.Added, f.Path)
			fmt.Fprintf(w, "--- router:%s (missing)\n+++ local:%s\n", remotePath, f.Path)
			for _, line := range strings.Split(string(f.Data), "\n") {
				fmt.Fprintf(w, "+%s\n", line)
			}
			continue
		}
		if normalize(remote) == normalize(f.Data) {
			continue
		}
		sum.Changed = append(sum.Changed, f.Path)
		fmt.Fprintf(w, "--- router:%s\n+++ local:%s\n", remotePath, f.Path)
		writeLineDiff(w, string(remote), string(f.Data))
	}
	return sum, nil
}

// normalize strips trailing whitespace and empty trailing lines — same
// rule as render_test.go so diff results align with the golden test.
func normalize(b []byte) string {
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	// drop trailing empties
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// writeLineDiff emits a simple unified-ish diff: common prefix / suffix
// are hidden, the divergent middle is printed as - (old) / + (new)
// blocks. Good enough for router config files which are small and
// typically change in a handful of places.
func writeLineDiff(w io.Writer, a, b string) {
	al := strings.Split(a, "\n")
	bl := strings.Split(b, "\n")
	// normalize trailing empties
	for len(al) > 0 && strings.TrimRight(al[len(al)-1], " \t\r") == "" {
		al = al[:len(al)-1]
	}
	for len(bl) > 0 && strings.TrimRight(bl[len(bl)-1], " \t\r") == "" {
		bl = bl[:len(bl)-1]
	}

	// Common prefix.
	p := 0
	for p < len(al) && p < len(bl) && al[p] == bl[p] {
		p++
	}
	// Common suffix.
	sA, sB := len(al), len(bl)
	for sA > p && sB > p && al[sA-1] == bl[sB-1] {
		sA--
		sB--
	}
	// Up to 2 lines of context before and after.
	ctxBefore := p
	if ctxBefore > 2 {
		ctxBefore = 2
	}
	for i := p - ctxBefore; i < p; i++ {
		fmt.Fprintf(w, " %s\n", al[i])
	}
	for i := p; i < sA; i++ {
		fmt.Fprintf(w, "-%s\n", al[i])
	}
	for i := p; i < sB; i++ {
		fmt.Fprintf(w, "+%s\n", bl[i])
	}
	after := len(al) - sA
	if after > 2 {
		after = 2
	}
	for i := sA; i < sA+after; i++ {
		fmt.Fprintf(w, " %s\n", al[i])
	}
}
