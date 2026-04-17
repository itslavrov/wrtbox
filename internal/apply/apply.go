// Package apply runs the push pipeline: render locally, upload to a
// staging dir on the router, validate, snapshot the live state, swap,
// reload services and post-check. On post-check failure it restores
// from the snapshot automatically.
package apply

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/itslavrov/wrtbox/internal/config"
	"github.com/itslavrov/wrtbox/internal/lists"
	"github.com/itslavrov/wrtbox/internal/render"
	"github.com/itslavrov/wrtbox/internal/ssh"
)

// Options tunes Run.
type Options struct {
	DryRun     bool
	BackupKeep int // default 5
	Writer     io.Writer
	Lists      lists.Provider
	// Now is injected so tests get deterministic timestamps.
	Now func() time.Time
}

// Result summarises one apply run.
type Result struct {
	Timestamp  string // ts used for staging/backup dirs
	StagingDir string // /root/wrtbox-staging/<ts>
	BackupDir  string // /root/wrtbox-backups/<ts>
	FileCount  int
	RolledBack bool
}

// Paths owned by wrtbox on the router. Anything outside this set is
// left untouched — wrtbox never removes files it did not create, only
// overwrites these.
var managedPaths = []string{
	"etc/config/network",
	"etc/config/firewall",
	"etc/config/dhcp",
	"etc/config/wireless",
	"etc/xray/config.json",
	"etc/xray/nft.conf",
	"etc/init.d/xray-rules",
	"etc/hotplug.d/iface/99-xray-rules",
	"usr/bin/update-antifilter.sh",
	"usr/bin/update-geosite.sh",
	"root/rollback.sh",
	"etc/crontabs/root",
}

// Reloads issued in order after swap.
var reloadCmds = []string{
	"/etc/init.d/network reload",
	"/etc/init.d/firewall reload",
	"/etc/init.d/xray restart 2>/dev/null || /etc/init.d/xray start 2>/dev/null || true",
	"wifi reload",
}

// Run is the main entry. It returns nil when the new config is in
// place AND post-checks pass, or when DryRun is set. On any failure
// after snapshot, it rolls back before returning.
func Run(ctx context.Context, exec ssh.Executor, cfg *config.Config, opts Options) (*Result, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.BackupKeep <= 0 {
		opts.BackupKeep = 5
	}
	w := opts.Writer
	if w == nil {
		w = io.Discard
	}
	logf := func(format string, a ...interface{}) {
		fmt.Fprintf(w, format+"\n", a...)
	}

	files, err := render.Render(ctx, cfg, render.Options{Lists: opts.Lists})
	if err != nil {
		return nil, fmt.Errorf("apply: render: %w", err)
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	ts := opts.Now().UTC().Format("20060102-150405")
	res := &Result{
		Timestamp:  ts,
		StagingDir: "/root/wrtbox-staging/" + ts,
		BackupDir:  "/root/wrtbox-backups/" + ts,
		FileCount:  len(files),
	}

	if opts.DryRun {
		logf("dry-run: would upload %d files to %s", len(files), res.StagingDir)
		for _, f := range files {
			logf("  + /%s (%d bytes, mode %o)", f.Path, len(f.Data), f.Mode)
		}
		logf("dry-run: would snapshot current state to %s", res.BackupDir)
		logf("dry-run: would swap %d managed paths and reload services", len(managedPaths))
		return res, nil
	}

	// 1. Sanity-check the remote.
	logf("[1/7] probing remote...")
	if _, _, err := exec.Run(ctx, "test -d /etc/config && test -d /etc/xray || mkdir -p /etc/xray"); err != nil {
		return res, fmt.Errorf("apply: probe: %w", err)
	}

	// 2. Upload to staging.
	logf("[2/7] uploading %d files to %s", len(files), res.StagingDir)
	for _, f := range files {
		dest := path.Join(res.StagingDir, f.Path)
		if err := exec.Upload(ctx, dest, f.Data, fs.FileMode(f.Mode)); err != nil {
			return res, fmt.Errorf("apply: upload %s: %w", dest, err)
		}
	}

	// 3. Validate staged configs.
	logf("[3/7] validating staged configs")
	if err := validateStaging(ctx, exec, res.StagingDir); err != nil {
		return res, fmt.Errorf("apply: validate: %w (staging left at %s for inspection)", err, res.StagingDir)
	}

	// 4. Snapshot current live state.
	logf("[4/7] snapshotting current state to %s", res.BackupDir)
	if err := snapshot(ctx, exec, res.BackupDir); err != nil {
		return res, fmt.Errorf("apply: snapshot: %w", err)
	}

	// 5. Swap: copy staged files over live paths.
	logf("[5/7] swapping %d managed files into place", len(files))
	if err := swap(ctx, exec, res.StagingDir, files); err != nil {
		// Attempt rollback immediately — swap may have partially
		// written before failing.
		logf("swap failed: %v — rolling back", err)
		_ = restore(ctx, exec, res.BackupDir)
		_, _, _ = exec.Run(ctx, strings.Join(reloadCmds, " && "))
		res.RolledBack = true
		return res, fmt.Errorf("apply: swap: %w (rolled back)", err)
	}

	// 6. Reload services.
	logf("[6/7] reloading services")
	if _, errOut, err := exec.Run(ctx, strings.Join(reloadCmds, " && ")); err != nil {
		logf("reload failed: %v — stderr: %s", err, trimStderr(errOut))
		_ = restore(ctx, exec, res.BackupDir)
		_, _, _ = exec.Run(ctx, strings.Join(reloadCmds, " && "))
		res.RolledBack = true
		return res, fmt.Errorf("apply: reload: %w (rolled back)", err)
	}

	// 7. Post-check: VPN reachability.
	logf("[7/7] post-check (VPN reachability)")
	if err := postCheck(ctx, exec, cfg); err != nil {
		logf("post-check failed: %v — rolling back", err)
		_ = restore(ctx, exec, res.BackupDir)
		_, _, _ = exec.Run(ctx, strings.Join(reloadCmds, " && "))
		res.RolledBack = true
		return res, fmt.Errorf("apply: post-check: %w (rolled back)", err)
	}

	// 8. Prune old backups (best-effort).
	if err := pruneBackups(ctx, exec, opts.BackupKeep); err != nil {
		logf("warn: backup prune failed: %v", err)
	}
	// Staging can go — it's already been swapped and snapshotted.
	_, _, _ = exec.Run(ctx, "rm -rf "+shellQuote(res.StagingDir))

	logf("apply complete (%d files, snapshot %s)", len(files), res.BackupDir)
	return res, nil
}

func validateStaging(ctx context.Context, exec ssh.Executor, stagingDir string) error {
	// UCI syntax check. `uci -c <dir> show` exits non-zero on parse
	// errors and prints them to stderr.
	cfgDir := path.Join(stagingDir, "etc/config")
	if _, errOut, err := exec.Run(ctx, "uci -c "+shellQuote(cfgDir)+" show >/dev/null"); err != nil {
		return fmt.Errorf("uci parse: %w — %s", err, trimStderr(errOut))
	}
	// Xray test. Recent xray (>=1.8) wants `-test -config <file>`;
	// older builds still accept `-test -c`. Try both, appending stderr
	// from both attempts so the final error message surfaces the real
	// problem (e.g. bad rule, invalid key) — not just "unknown flag".
	xrayJSON := path.Join(stagingDir, "etc/xray/config.json")
	xrayCmd := ": >/tmp/wrtbox-xray.err;" +
		" { xray -test -config " + shellQuote(xrayJSON) + " >>/tmp/wrtbox-xray.err 2>&1" +
		" || xray -test -c " + shellQuote(xrayJSON) + " >>/tmp/wrtbox-xray.err 2>&1; }" +
		" || { cat /tmp/wrtbox-xray.err >&2; exit 1; }"
	if _, errOut, err := exec.Run(ctx, xrayCmd); err != nil {
		return fmt.Errorf("xray test: %w — %s", err, trimStderr(errOut))
	}
	return nil
}

func snapshot(ctx context.Context, exec ssh.Executor, backupDir string) error {
	if _, _, err := exec.Run(ctx, "mkdir -p "+shellQuote(backupDir)); err != nil {
		return err
	}
	for _, rel := range managedPaths {
		src := "/" + rel
		dst := path.Join(backupDir, rel)
		// `cp --parents` is not guaranteed on busybox. Use mkdir+cp.
		cmd := fmt.Sprintf("mkdir -p %s && if [ -e %s ]; then cp -a %s %s; fi",
			shellQuote(path.Dir(dst)), shellQuote(src), shellQuote(src), shellQuote(dst))
		if _, errOut, err := exec.Run(ctx, cmd); err != nil {
			return fmt.Errorf("snapshot %s: %w — %s", rel, err, trimStderr(errOut))
		}
	}
	return nil
}

func swap(ctx context.Context, exec ssh.Executor, stagingDir string, files []render.File) error {
	for _, f := range files {
		src := path.Join(stagingDir, f.Path)
		dst := "/" + f.Path
		cmd := fmt.Sprintf("mkdir -p %s && cp -a %s %s",
			shellQuote(path.Dir(dst)), shellQuote(src), shellQuote(dst))
		if _, errOut, err := exec.Run(ctx, cmd); err != nil {
			return fmt.Errorf("cp %s: %w — %s", f.Path, err, trimStderr(errOut))
		}
	}
	return nil
}

// Restore is exported so rollback.go can reuse it.
func Restore(ctx context.Context, exec ssh.Executor, backupDir string) error {
	return restore(ctx, exec, backupDir)
}

func restore(ctx context.Context, exec ssh.Executor, backupDir string) error {
	for _, rel := range managedPaths {
		src := path.Join(backupDir, rel)
		dst := "/" + rel
		cmd := fmt.Sprintf("if [ -e %s ]; then mkdir -p %s && cp -a %s %s; fi",
			shellQuote(src), shellQuote(path.Dir(dst)), shellQuote(src), shellQuote(dst))
		if _, _, err := exec.Run(ctx, cmd); err != nil {
			return fmt.Errorf("restore %s: %w", rel, err)
		}
	}
	return nil
}

func postCheck(ctx context.Context, exec ssh.Executor, cfg *config.Config) error {
	// xray process is alive.
	if _, _, err := exec.Run(ctx, "pidof xray >/dev/null"); err != nil {
		return fmt.Errorf("xray not running")
	}
	// Give routing a moment to settle, then ping through vpnlan if
	// configured. If vpnlan is absent we skip the network check.
	if cfg.Spec.VPNLan == nil {
		return nil
	}
	gw := firstIP(cfg.Spec.VPNLan.IPAddr)
	if gw == "" {
		return nil
	}
	cmd := fmt.Sprintf("sleep 3 && ping -I %s -c 2 -W 3 1.1.1.1 >/dev/null", shellQuote(gw))
	if _, errOut, err := exec.Run(ctx, cmd); err != nil {
		return fmt.Errorf("ping via vpnlan: %w — %s", err, trimStderr(errOut))
	}
	return nil
}

// ListBackups returns backup timestamps sorted newest-first.
func ListBackups(ctx context.Context, exec ssh.Executor) ([]string, error) {
	stdout, _, err := exec.Run(ctx, "ls -1 /root/wrtbox-backups 2>/dev/null || true")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(string(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out, nil
}

func pruneBackups(ctx context.Context, exec ssh.Executor, keep int) error {
	ts, err := ListBackups(ctx, exec)
	if err != nil {
		return err
	}
	if len(ts) <= keep {
		return nil
	}
	for _, old := range ts[keep:] {
		if _, _, err := exec.Run(ctx, "rm -rf /root/wrtbox-backups/"+shellQuote(old)); err != nil {
			return err
		}
	}
	return nil
}

func firstIP(cidr string) string {
	// "10.10.0.1/24" → "10.10.0.1"
	if i := strings.Index(cidr, "/"); i >= 0 {
		return cidr[:i]
	}
	return cidr
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func trimStderr(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 240 {
		s = s[:240] + "...(truncated)"
	}
	return s
}
