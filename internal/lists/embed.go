package lists

import (
	"context"
	"embed"
	"fmt"
	"strings"
)

//go:embed assets/*.txt
var assets embed.FS

// EmbedProvider serves compiled-in list snapshots. Used for deterministic
// tests and as safe offline defaults (e.g., the antifilter.download ipsum
// snapshot that reproduces the user's backup byte-for-byte).
type EmbedProvider struct{}

// NewEmbedProvider returns a ready-to-use embed-backed provider.
func NewEmbedProvider() *EmbedProvider { return &EmbedProvider{} }

// Fetch implements Provider.
func (p *EmbedProvider) Fetch(_ context.Context, ref string) ([]string, error) {
	name := strings.TrimPrefix(ref, "embed:")
	if name == "" {
		return nil, fmt.Errorf("lists: embed ref has empty name")
	}
	data, err := assets.ReadFile("assets/" + name + ".txt")
	if err != nil {
		return nil, fmt.Errorf("lists: embed %s: %w", name, err)
	}
	return readLines(strings.NewReader(string(data)))
}
