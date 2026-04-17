package hosts

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// withHome points UserHomeDir at a temp dir for the duration of the test.
func withHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func TestLoad_MissingFileReturnsEmptyRegistry(t *testing.T) {
	withHome(t)
	reg, err := Load("/nonexistent/path/hosts.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(reg.entries) != 0 {
		t.Errorf("expected empty registry")
	}
}

func TestResolve_BareHostFlag(t *testing.T) {
	home := withHome(t)
	// Provide a default key file so fill() picks it up.
	writeFile(t, filepath.Join(home, ".ssh", "id_ed25519"), "fake-key")

	reg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	rt, err := reg.Resolve("admin@192.168.1.1:2222")
	if err != nil {
		t.Fatal(err)
	}
	if rt.User != "admin" {
		t.Errorf("User = %q, want admin", rt.User)
	}
	if rt.Host != "192.168.1.1" {
		t.Errorf("Host = %q, want 192.168.1.1", rt.Host)
	}
	if rt.Port != 2222 {
		t.Errorf("Port = %d, want 2222", rt.Port)
	}
	if rt.Key == "" {
		t.Errorf("Key should default to ~/.ssh/id_ed25519")
	}
}

func TestResolve_FromHostsYAML(t *testing.T) {
	home := withHome(t)
	writeFile(t, filepath.Join(home, ".ssh", "id_ed25519"), "fake-key")

	hostsYAML := filepath.Join(home, ".config", "wrtbox", "hosts.yaml")
	writeFile(t, hostsYAML, `routers:
  home:
    host: 10.0.0.1
    user: root
    port: 22
`)
	reg, err := Load(hostsYAML)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := reg.Resolve("home")
	if err != nil {
		t.Fatal(err)
	}
	if rt.Host != "10.0.0.1" || rt.User != "root" || rt.Port != 22 {
		t.Errorf("bad resolve: %+v", rt)
	}
}

func TestResolve_SSHConfigAlias(t *testing.T) {
	home := withHome(t)
	writeFile(t, filepath.Join(home, ".ssh", "id_ed25519"), "fake-key")
	writeFile(t, filepath.Join(home, ".ssh", "config"), `Host myrouter
  HostName 192.168.8.1
  User admin
  Port 2200
  IdentityFile ~/.ssh/id_ed25519
`)
	reg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	rt, err := reg.Resolve("myrouter")
	if err != nil {
		t.Fatal(err)
	}
	if rt.Host != "192.168.8.1" {
		t.Errorf("Host = %q", rt.Host)
	}
	if rt.User != "admin" {
		t.Errorf("User = %q", rt.User)
	}
	if rt.Port != 2200 {
		t.Errorf("Port = %d", rt.Port)
	}
}

func TestOverride_HostFlagReplaces(t *testing.T) {
	home := withHome(t)
	writeFile(t, filepath.Join(home, ".ssh", "id_ed25519"), "fake-key")
	reg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	rt, err := reg.Resolve("home")
	if err != nil {
		t.Fatal(err)
	}
	rt, err = reg.Override(rt, "root@172.16.0.1:22", "")
	if err != nil {
		t.Fatal(err)
	}
	if rt.Host != "172.16.0.1" || rt.User != "root" {
		t.Errorf("override: %+v", rt)
	}
}
