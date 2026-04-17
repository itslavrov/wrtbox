package diff

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/itslavrov/wrtbox/internal/config"
	"github.com/itslavrov/wrtbox/internal/render"
	"github.com/itslavrov/wrtbox/internal/ssh/sshmock"
)

const fixtureConfig = "../render/testdata/gl-mt6000/wrtbox.yaml"

func TestRun_EmptyRouterMeansFullAdd(t *testing.T) {
	cfg, err := config.Load(fixtureConfig)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	exec := sshmock.New()
	var buf bytes.Buffer
	sum, err := Run(context.Background(), exec, cfg, Options{Writer: &buf})
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(sum.Added) < 10 {
		t.Errorf("expected most files as Added on empty router; got %d", len(sum.Added))
	}
	if !strings.Contains(buf.String(), "+++ local:etc/xray/config.json") {
		t.Errorf("output missing xray config header:\n%s", buf.String())
	}
}

func TestRun_IdenticalRouterMeansNoDiff(t *testing.T) {
	cfg, err := config.Load(fixtureConfig)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	files, err := render.Render(context.Background(), cfg, render.Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	exec := sshmock.New()
	for _, f := range files {
		exec.Files["/"+f.Path] = f.Data
	}
	var buf bytes.Buffer
	sum, err := Run(context.Background(), exec, cfg, Options{Writer: &buf})
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(sum.Changed) != 0 || len(sum.Added) != 0 {
		t.Errorf("expected no diff, got changed=%d added=%d\n%s",
			len(sum.Changed), len(sum.Added), buf.String())
	}
}

func TestRun_SingleLineChangeShowsMinusPlus(t *testing.T) {
	cfg, err := config.Load(fixtureConfig)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	files, err := render.Render(context.Background(), cfg, render.Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	exec := sshmock.New()
	// Populate router with a mutated version of network: swap one
	// value so the normaliser still sees a diff.
	for _, f := range files {
		if f.Path == "etc/config/network" {
			modified := strings.Replace(string(f.Data), "192.168.1.1", "192.168.9.9", 1)
			exec.Files["/"+f.Path] = []byte(modified)
		} else {
			exec.Files["/"+f.Path] = f.Data
		}
	}
	var buf bytes.Buffer
	sum, err := Run(context.Background(), exec, cfg, Options{Writer: &buf})
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(sum.Changed) != 1 || sum.Changed[0] != "etc/config/network" {
		t.Errorf("expected exactly network changed, got %v", sum.Changed)
	}
	out := buf.String()
	if !strings.Contains(out, "-") || !strings.Contains(out, "+") {
		t.Errorf("expected -/+ lines in output:\n%s", out)
	}
}
