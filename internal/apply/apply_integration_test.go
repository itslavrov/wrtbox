//go:build integration

package apply_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/itslavrov/wrtbox/internal/apply"
	"github.com/itslavrov/wrtbox/internal/config"
	"github.com/itslavrov/wrtbox/internal/ssh"
)

// Integration test: runs the full apply pipeline against a real OpenWrt
// emulator VM with a real xray-core binary at /usr/bin/xray. Validate
// stage runs real `xray -test -config` against the rendered config.
// Post-check is expected to fail ("xray not running" — the emu ships no
// /etc/init.d/xray, so nothing starts the daemon) and auto-rollback; we
// assert the rollback flag, the error shape, and that wrtbox left the
// router in a clean state.
//
// Requires env:
//
//	WRTBOX_EMU_HOST  — IP or hostname of the OpenWrt emu VM
//	WRTBOX_EMU_KEY   — path to the private SSH key (default ~/.ssh/wrtbox-emu)
//	WRTBOX_EMU_USER  — SSH user (default root)
//
// Skipped cleanly when WRTBOX_EMU_HOST is not set.
func TestIntegrationApplyRollback(t *testing.T) {
	host := os.Getenv("WRTBOX_EMU_HOST")
	if host == "" {
		t.Skip("WRTBOX_EMU_HOST not set — skipping integration test")
	}
	keyPath := os.Getenv("WRTBOX_EMU_KEY")
	if keyPath == "" {
		home, _ := os.UserHomeDir()
		keyPath = filepath.Join(home, ".ssh", "wrtbox-emu")
	}
	user := os.Getenv("WRTBOX_EMU_USER")
	if user == "" {
		user = "root"
	}

	// known_hosts: use a scratch file so the test never touches the
	// caller's default one and TOFU is safe.
	kh, err := os.CreateTemp("", "wrtbox-integ-kh-*")
	if err != nil {
		t.Fatalf("temp known_hosts: %v", err)
	}
	defer os.Remove(kh.Name())
	_ = kh.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client, err := ssh.Dial(ctx, ssh.DialOptions{
		Host:             host,
		Port:             22,
		User:             user,
		KeyPath:          keyPath,
		KnownHostsPath:   kh.Name(),
		AcceptNewHostKey: true,
		ConnectTimeout:   15 * time.Second,
	})
	if err != nil {
		t.Fatalf("ssh dial: %v", err)
	}

	// Always clean staging/backups so repeated runs on the same VM stay
	// tidy, then close. Order matters: Run needs the client alive.
	t.Cleanup(func() {
		_, _, _ = client.Run(context.Background(), "rm -rf /root/wrtbox-staging /root/wrtbox-backups")
		_ = client.Close()
	})

	// Capture baseline of a managed path; we will assert it is unchanged
	// after the rollback.
	baselineNet, _, err := client.Run(ctx, "cat /etc/config/network")
	if err != nil {
		t.Fatalf("read baseline /etc/config/network: %v", err)
	}

	// Find the x86_64 emu example relative to this test file.
	cfgPath := filepath.Join("..", "..", "examples", "x86_64-emu.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load example config %s: %v", cfgPath, err)
	}

	var log strings.Builder
	res, err := apply.Run(ctx, client, cfg, apply.Options{
		Writer:     &log,
		BackupKeep: 2,
	})

	if err == nil {
		t.Fatalf("expected apply to fail at post-check (no xray daemon running on emu), got nil error\nlog:\n%s", log.String())
	}
	if !strings.Contains(err.Error(), "post-check") {
		t.Fatalf("error should mention post-check, got: %v", err)
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("error should mention rolled back, got: %v", err)
	}
	if res == nil || !res.RolledBack {
		t.Fatalf("Result.RolledBack should be true, got res=%+v", res)
	}

	// Verify rollback actually restored /etc/config/network.
	afterNet, _, err := client.Run(ctx, "cat /etc/config/network")
	if err != nil {
		t.Fatalf("read /etc/config/network after rollback: %v", err)
	}
	if string(afterNet) != string(baselineNet) {
		t.Errorf("/etc/config/network changed after rollback\nbaseline:\n%s\nafter:\n%s",
			baselineNet, afterNet)
	}

	// Sanity: xray (if still running after rollback reload) and router
	// remain reachable via SSH — we already exec'd a command above.
	t.Logf("apply pipeline log:\n%s", log.String())
}
