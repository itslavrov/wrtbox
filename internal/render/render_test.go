package render_test

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/itslavrov/wrtbox/internal/config"
	"github.com/itslavrov/wrtbox/internal/render"
)

var update = flag.Bool("update", false, "regenerate golden testdata files")

// TestRenderGolden is the Phase 1 Definition-of-Done check: render a
// fixture resembling a real GL-MT6000 setup and assert the output tree
// matches the frozen goldens byte-for-byte (after whitespace
// normalisation).
//
// Run `go test ./internal/render -update` after an intentional render
// change to refresh the goldens.
func TestRenderGolden(t *testing.T) {
	const fixtureDir = "testdata/gl-mt6000"
	cfgPath := filepath.Join(fixtureDir, "wrtbox.yaml")
	expectedRoot := filepath.Join(fixtureDir, "expected")

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	files, err := render.Render(context.Background(), cfg, render.Options{})
	if err != nil {
		t.Fatalf("render.Render: %v", err)
	}

	got := make(map[string][]byte, len(files))
	for _, f := range files {
		got[f.Path] = f.Data
	}

	if *update {
		if err := os.RemoveAll(expectedRoot); err != nil {
			t.Fatalf("rm expected: %v", err)
		}
		for _, f := range files {
			full := filepath.Join(expectedRoot, f.Path)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(full, f.Data, 0o644); err != nil {
				t.Fatalf("write %s: %v", full, err)
			}
		}
		t.Logf("wrote %d golden files under %s", len(files), expectedRoot)
		return
	}

	// Walk the expected tree; flag missing and surplus files.
	seen := make(map[string]bool, len(files))
	err = filepath.Walk(expectedRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(expectedRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		seen[rel] = true
		exp, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		act, ok := got[rel]
		if !ok {
			t.Errorf("missing from render: %s", rel)
			return nil
		}
		if normalize(exp) != normalize(act) {
			t.Errorf("%s differs (len exp=%d act=%d)\n--- expected (first 400B) ---\n%s\n--- actual (first 400B) ---\n%s",
				rel, len(exp), len(act), headBytes(exp, 400), headBytes(act, 400))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk expected: %v", err)
	}
	for path := range got {
		if !seen[path] {
			t.Errorf("unexpected in render: %s", path)
		}
	}
}

// normalize folds whitespace the same way on both sides: strips CRs,
// collapses runs of horizontal whitespace, trims trailing space per
// line, and ensures exactly one trailing newline.
func normalize(b []byte) string {
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	s = trailing.ReplaceAllString(s, "")
	return strings.TrimRight(s, "\n") + "\n"
}

var trailing = regexp.MustCompile(`[ \t]+\n`)

func headBytes(b []byte, n int) string {
	if len(b) < n {
		return string(b)
	}
	return string(b[:n])
}
