package lists_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/itslavrov/wrtbox/internal/lists"
)

func TestEmbedAntifilter(t *testing.T) {
	r := lists.NewRegistry()
	entries, err := r.Fetch(context.Background(), "embed:antifilter-ipsum")
	if err != nil {
		t.Fatalf("fetch embed: %v", err)
	}
	if len(entries) < 1000 {
		t.Fatalf("expected a substantial antifilter snapshot, got %d entries", len(entries))
	}
}

func TestFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "list.txt")
	if err := os.WriteFile(path, []byte("# comment\n\n1.2.3.0/24\n4.5.6.0/24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := lists.NewRegistry()
	entries, err := r.Fetch(context.Background(), "file:"+path)
	if err != nil {
		t.Fatalf("fetch file: %v", err)
	}
	if len(entries) != 2 || entries[0] != "1.2.3.0/24" {
		t.Fatalf("unexpected entries: %v", entries)
	}
}

func TestUnknownScheme(t *testing.T) {
	r := lists.NewRegistry()
	_, err := r.Fetch(context.Background(), "ftp://example/list")
	if err == nil {
		t.Fatal("expected unknown-scheme error")
	}
}
