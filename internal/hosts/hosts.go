// Package hosts loads router endpoint definitions from
// ~/.config/wrtbox/hosts.yaml and merges them with ~/.ssh/config so
// users can keep SSH details in one familiar place.
package hosts

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kevinburke/ssh_config"
	"gopkg.in/yaml.v3"
)

// Router is a fully resolved endpoint: host, port, user, key and
// optional known_hosts override.
type Router struct {
	Name       string
	Host       string
	Port       int
	User       string
	Key        string // absolute path
	KnownHosts string // absolute path (empty = default)
}

// fileDoc is the YAML wire format. Intentionally small — users are
// expected to keep the richer config in ~/.ssh/config.
type fileDoc struct {
	Routers map[string]entry `yaml:"routers"`
}

type entry struct {
	Host       string `yaml:"host"`        // alias from ~/.ssh/config, or bare host / user@host[:port]
	User       string `yaml:"user"`        // override
	Port       int    `yaml:"port"`        // override
	Key        string `yaml:"key"`         // override — ~ is expanded
	KnownHosts string `yaml:"known_hosts"` // override
}

// Registry is a loaded hosts.yaml plus access to ~/.ssh/config.
type Registry struct {
	entries  map[string]entry
	sshConf  *ssh_config.Config
	homeDir  string
	filePath string
}

// DefaultPath returns ~/.config/wrtbox/hosts.yaml.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "wrtbox", "hosts.yaml"), nil
}

// Load reads path (or returns an empty Registry if path does not
// exist). ~/.ssh/config is read opportunistically — a missing file
// is not an error.
func Load(path string) (*Registry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	reg := &Registry{entries: map[string]entry{}, homeDir: home, filePath: path}

	if path != "" {
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			var doc fileDoc
			dec := yaml.NewDecoder(strings.NewReader(string(data)))
			dec.KnownFields(true)
			if err := dec.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
				return nil, fmt.Errorf("hosts: parse %s: %w", path, err)
			}
			reg.entries = doc.Routers
		case errors.Is(err, os.ErrNotExist):
			// empty registry — callers may still pass --host
		default:
			return nil, fmt.Errorf("hosts: read %s: %w", path, err)
		}
	}

	if f, err := os.Open(filepath.Join(home, ".ssh", "config")); err == nil {
		defer f.Close()
		if cfg, err := ssh_config.Decode(f); err == nil {
			reg.sshConf = cfg
		}
	}

	return reg, nil
}

// Resolve returns the Router for name. If name is not in hosts.yaml it
// is treated as a raw alias / user@host[:port] passed via --host.
func (r *Registry) Resolve(name string) (Router, error) {
	e, inYAML := r.entries[name]
	if !inYAML {
		e = entry{Host: name}
	}
	rt := Router{Name: name}
	if err := r.fill(&rt, e); err != nil {
		return Router{}, err
	}
	if rt.Host == "" {
		return Router{}, fmt.Errorf("hosts: %q resolved to empty host", name)
	}
	return rt, nil
}

// Override applies CLI flag overrides on top of a resolved Router.
func (r *Registry) Override(rt Router, hostFlag, keyFlag string) (Router, error) {
	if hostFlag != "" {
		rt.Name = hostFlag
		rt.Host = ""
		rt.Port = 0
		rt.User = ""
		if err := r.fill(&rt, entry{Host: hostFlag}); err != nil {
			return Router{}, err
		}
	}
	if keyFlag != "" {
		rt.Key = expandHome(keyFlag, r.homeDir)
	}
	return rt, nil
}

func (r *Registry) fill(rt *Router, e entry) error {
	// Split host into (user, host, port) if in user@host[:port] form.
	host, user, port := splitTarget(e.Host)

	// Apply entry-level overrides first.
	if e.User != "" {
		user = e.User
	}
	if e.Port != 0 {
		port = e.Port
	}
	rt.Key = expandHome(e.Key, r.homeDir)
	rt.KnownHosts = expandHome(e.KnownHosts, r.homeDir)

	// Pull anything still missing from ~/.ssh/config, keyed on the
	// alias BEFORE user@ stripping (so `Host r1` with HostName works).
	alias := host
	if r.sshConf != nil {
		if v, _ := r.sshConf.Get(alias, "HostName"); v != "" {
			rt.Host = v
		}
		if rt.Host == "" {
			rt.Host = host
		}
		if user == "" {
			if v, _ := r.sshConf.Get(alias, "User"); v != "" {
				user = v
			}
		}
		if port == 0 {
			if v, _ := r.sshConf.Get(alias, "Port"); v != "" {
				if p, err := strconv.Atoi(v); err == nil {
					port = p
				}
			}
		}
		if rt.Key == "" {
			if v, _ := r.sshConf.Get(alias, "IdentityFile"); v != "" {
				rt.Key = expandHome(v, r.homeDir)
			}
		}
	} else {
		rt.Host = host
	}

	if user == "" {
		user = "root"
	}
	if port == 0 {
		port = 22
	}
	if rt.Key == "" {
		for _, cand := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
			p := filepath.Join(r.homeDir, ".ssh", cand)
			if _, err := os.Stat(p); err == nil {
				rt.Key = p
				break
			}
		}
	}
	rt.User = user
	rt.Port = port
	return nil
}

// splitTarget parses forms: "host", "user@host", "host:port",
// "user@host:port". Returns (host, user, port).
func splitTarget(s string) (host, user string, port int) {
	if i := strings.Index(s, "@"); i >= 0 {
		user = s[:i]
		s = s[i+1:]
	}
	if i := strings.LastIndex(s, ":"); i >= 0 {
		if p, err := strconv.Atoi(s[i+1:]); err == nil {
			port = p
			s = s[:i]
		}
	}
	host = s
	return
}

func expandHome(p, home string) string {
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	if p == "~" {
		return home
	}
	return p
}
