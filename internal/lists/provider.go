// Package lists provides a pluggable source abstraction for routing lists
// (CIDR blocks, domain lists) that wrtbox bakes into the generated xray
// config at render time.
//
// A source is addressed by a URI-like prefix:
//
//	embed:<name>            → compiled-in list (see assets.go)
//	file:/absolute/path     → local file on the render host
//	http(s)://example/list  → remote HTTP fetch
//
// Keeping this behind a small interface lets render be deterministic in
// tests (embed) and live in production (file on the router, http for
// ad-hoc pulls) without the routing code caring which is which.
package lists

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Provider resolves a list reference to a slice of non-empty, trimmed entries.
// Comment lines starting with '#' and blank lines are dropped.
type Provider interface {
	Fetch(ctx context.Context, ref string) ([]string, error)
}

// Registry dispatches refs by prefix to individual providers.
type Registry struct {
	Embed *EmbedProvider
	File  *FileProvider
	HTTP  *HTTPProvider
}

// NewRegistry returns a Registry populated with default providers.
func NewRegistry() *Registry {
	return &Registry{
		Embed: NewEmbedProvider(),
		File:  &FileProvider{},
		HTTP:  &HTTPProvider{Client: &http.Client{Timeout: 60 * time.Second}},
	}
}

// Fetch routes ref to the correct underlying provider based on its prefix.
func (r *Registry) Fetch(ctx context.Context, ref string) ([]string, error) {
	switch {
	case strings.HasPrefix(ref, "embed:"):
		return r.Embed.Fetch(ctx, ref)
	case strings.HasPrefix(ref, "file:"):
		return r.File.Fetch(ctx, ref)
	case strings.HasPrefix(ref, "http://"), strings.HasPrefix(ref, "https://"):
		return r.HTTP.Fetch(ctx, ref)
	default:
		return nil, fmt.Errorf("lists: unsupported ref scheme: %q", ref)
	}
}

// FileProvider reads a list from a local file. The prefix `file:` is stripped;
// everything after is treated as an absolute or relative path.
type FileProvider struct{}

// Fetch implements Provider.
func (p *FileProvider) Fetch(_ context.Context, ref string) ([]string, error) {
	path := strings.TrimPrefix(ref, "file:")
	if path == "" {
		return nil, errors.New("lists: file ref has empty path")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("lists: open %s: %w", path, err)
	}
	defer f.Close()
	return readLines(f)
}

// HTTPProvider fetches a list over HTTP(s).
type HTTPProvider struct {
	Client *http.Client
}

// Fetch implements Provider.
func (p *HTTPProvider) Fetch(ctx context.Context, ref string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref, nil)
	if err != nil {
		return nil, fmt.Errorf("lists: build request: %w", err)
	}
	req.Header.Set("User-Agent", "wrtbox/0.1 (+https://github.com/itslavrov/wrtbox)")
	resp, err := p.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lists: fetch %s: %w", ref, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("lists: fetch %s: HTTP %d", ref, resp.StatusCode)
	}
	return readLines(resp.Body)
}

func readLines(r io.Reader) ([]string, error) {
	out := make([]string, 0, 1024)
	sc := bufio.NewScanner(r)
	// Allow large lines (some lists pack entries per-line but some are long).
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("lists: read: %w", err)
	}
	return out, nil
}
