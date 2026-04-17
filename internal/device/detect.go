package device

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/itslavrov/wrtbox/internal/ssh"
)

// BoardInfo mirrors the fields from `ubus call system board` we consume.
// We take the JSON verbatim and decide the wrtbox model label in Go so
// unit tests can be written against canned JSON without live ubus.
type BoardInfo struct {
	Kernel    string   `json:"kernel"`
	Hostname  string   `json:"hostname"`
	System    string   `json:"system"`
	Model     string   `json:"model"`
	BoardName string   `json:"board_name"`
	Release   *Release `json:"release,omitempty"`
}

// Release is the nested release object.
type Release struct {
	Distribution string `json:"distribution"`
	Version      string `json:"version"`
	Target       string `json:"target"`
	Revision     string `json:"revision"`
}

// Detection is the structured result of auto-detecting a device.
type Detection struct {
	Board BoardInfo // raw ubus output
	Model string    // mapped wrtbox model key (first-class or "generic")
}

// Detect runs `ubus call system board` on the remote and returns a
// BoardInfo + mapped wrtbox model key. Unknown boards map to "generic"
// — the caller is expected to fill spec.device.overrides.
func Detect(ctx context.Context, exec ssh.Executor) (*Detection, error) {
	stdout, errOut, err := exec.Run(ctx, "ubus call system board")
	if err != nil {
		return nil, fmt.Errorf("ubus call system board: %w — %s", err, strings.TrimSpace(string(errOut)))
	}
	var info BoardInfo
	if err := json.Unmarshal(stdout, &info); err != nil {
		return nil, fmt.Errorf("parse ubus JSON: %w", err)
	}
	return &Detection{Board: info, Model: MapBoardToModel(info)}, nil
}

// MapBoardToModel translates a BoardInfo to a wrtbox model key. Kept as
// a pure function so it is trivially testable without ubus.
func MapBoardToModel(info BoardInfo) string {
	name := strings.ToLower(info.BoardName)
	target := ""
	if info.Release != nil {
		target = strings.ToLower(info.Release.Target)
	}

	switch {
	case name == "glinet,mt6000" || strings.Contains(name, "gl-mt6000"):
		return "gl-mt6000"
	case strings.HasPrefix(name, "x86,") || strings.HasPrefix(target, "x86/"):
		return "x86_64"
	default:
		return "generic"
	}
}
