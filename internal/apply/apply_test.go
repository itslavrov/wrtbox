package apply

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/itslavrov/wrtbox/internal/config"
	"github.com/itslavrov/wrtbox/internal/ssh/sshmock"
)

const fixtureConfig = "../render/testdata/gl-mt6000/wrtbox.yaml"

func fixedNow() time.Time { return time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC) }

func loadFixture(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load(fixtureConfig)
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	return cfg
}

func TestDryRun_NoExec(t *testing.T) {
	cfg := loadFixture(t)
	var buf bytes.Buffer
	res, err := Run(context.Background(), nil, cfg, Options{
		DryRun: true,
		Writer: &buf,
		Now:    fixedNow,
	})
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if res.Timestamp != "20260417-120000" {
		t.Errorf("timestamp = %q, want 20260417-120000", res.Timestamp)
	}
	if res.FileCount < 10 {
		t.Errorf("FileCount = %d, want >= 10", res.FileCount)
	}
	got := buf.String()
	if !strings.Contains(got, "dry-run: would upload") {
		t.Errorf("dry-run output missing upload line:\n%s", got)
	}
	if !strings.Contains(got, "/etc/xray/config.json") {
		t.Errorf("dry-run output missing xray path:\n%s", got)
	}
}

func TestRun_HappyPath(t *testing.T) {
	cfg := loadFixture(t)
	exec := sshmock.New()
	// Simulate `pidof xray` returning a pid.
	exec.RunFunc = func(_ context.Context, cmd string) ([]byte, []byte, error) {
		if strings.Contains(cmd, "pidof xray") {
			return []byte("1234\n"), nil, nil
		}
		return nil, nil, nil
	}
	var buf bytes.Buffer
	res, err := Run(context.Background(), exec, cfg, Options{
		Writer: &buf,
		Now:    fixedNow,
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.RolledBack {
		t.Fatalf("unexpected rollback; output:\n%s", buf.String())
	}
	// Uploaded files should live under the staging dir.
	foundNetwork := false
	for p := range exec.Files {
		if strings.HasSuffix(p, "etc/config/network") && strings.Contains(p, res.StagingDir) {
			foundNetwork = true
		}
	}
	if !foundNetwork {
		t.Errorf("network config not uploaded to staging; files: %v", keys(exec.Files))
	}
	// Reload should have run.
	reloadSeen := false
	for _, c := range exec.Commands {
		if strings.Contains(c, "/etc/init.d/network reload") {
			reloadSeen = true
		}
	}
	if !reloadSeen {
		t.Errorf("network reload never issued; commands: %v", exec.Commands)
	}
}

func TestRun_XrayTestFailsAbortsBeforeSnapshot(t *testing.T) {
	cfg := loadFixture(t)
	exec := sshmock.New()
	exec.RunFunc = func(_ context.Context, cmd string) ([]byte, []byte, error) {
		if strings.Contains(cmd, "xray") && strings.Contains(cmd, "-test") {
			return nil, []byte("bad config"), &execError{msg: "exit 1"}
		}
		return nil, nil, nil
	}
	res, err := Run(context.Background(), exec, cfg, Options{Now: fixedNow})
	if err == nil {
		t.Fatal("expected xray test failure")
	}
	if res.RolledBack {
		t.Errorf("should not roll back — snapshot never taken")
	}
	// No snapshot command should have been issued.
	for _, c := range exec.Commands {
		if strings.Contains(c, "/root/wrtbox-backups/") {
			t.Errorf("snapshot was taken despite validate failure: %q", c)
		}
	}
}

func TestRun_PostCheckFailTriggersRollback(t *testing.T) {
	cfg := loadFixture(t)
	exec := sshmock.New()
	exec.RunFunc = func(_ context.Context, cmd string) ([]byte, []byte, error) {
		if strings.Contains(cmd, "pidof xray") {
			return nil, nil, &execError{msg: "exit 1"}
		}
		return nil, nil, nil
	}
	res, err := Run(context.Background(), exec, cfg, Options{Now: fixedNow})
	if err == nil {
		t.Fatal("expected post-check failure")
	}
	if !res.RolledBack {
		t.Errorf("post-check failure should trigger rollback")
	}
}

type execError struct{ msg string }

func (e *execError) Error() string { return e.msg }

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
