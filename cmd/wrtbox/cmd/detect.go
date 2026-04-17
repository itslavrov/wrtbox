package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/itslavrov/wrtbox/internal/device"
)

func newDetectCmd() *cobra.Command {
	var (
		remote  remoteFlags
		rawJSON bool
	)
	c := &cobra.Command{
		Use:   "detect",
		Short: "Auto-detect OpenWrt device via ubus and print a YAML device: block",
		Long: `Runs ` + "`ubus call system board`" + ` on the remote router over SSH and
prints a ready-to-paste spec.device block. If the board is a first-class
wrtbox target (gl-mt6000, x86_64) the profile is set directly; unknown
boards map to "generic" with overrides scaffolded from the live board.

Requires either --router (from hosts.yaml) or --host to reach the router.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			cli, _, err := remote.dial(ctx)
			if err != nil {
				return err
			}
			defer cli.Close()

			det, err := device.Detect(ctx, cli)
			if err != nil {
				return err
			}

			if rawJSON {
				b, _ := json.MarshalIndent(det, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
				return nil
			}

			fmt.Fprintln(cmd.OutOrStdout(), renderDetectYAML(det))
			return nil
		},
	}
	remote.bind(c)
	c.Flags().BoolVar(&rawJSON, "json", false, "emit raw JSON (board + mapped model) instead of YAML snippet")
	return c
}

// renderDetectYAML turns a Detection into a paste-ready YAML snippet.
// Built by hand (not via yaml.Marshal) so comments document intent and
// field order matches what a user would write themselves.
func renderDetectYAML(det *device.Detection) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Detected: %s (board_name=%s, target=%s)\n",
		det.Board.Model, det.Board.BoardName, targetOf(det.Board))
	fmt.Fprintln(&b, "spec:")
	fmt.Fprintln(&b, "  device:")
	fmt.Fprintf(&b, "    model: %s\n", det.Model)

	if det.Model != "generic" {
		return b.String()
	}

	// Scaffold overrides for unknown boards — safe guesses that users
	// should verify. We deliberately do NOT probe wifi paths over SSH
	// (requires parsing `iw phy` output across vendor kernels); the
	// user fills them in from `find /sys/devices -name 'phy*'`.
	fmt.Fprintln(&b, "    overrides:")
	fmt.Fprintln(&b, "      # Fill these in after checking on the router:")
	fmt.Fprintln(&b, "      #   ip -br link         — list NICs, pick the upstream one")
	fmt.Fprintln(&b, "      #   find /sys/devices -name 'phy*' -type d")
	fmt.Fprintln(&b, "      #   opkg list-installed | grep -E 'xray|nft-tproxy'")
	fmt.Fprintln(&b, "      wan_interface: eth1")
	fmt.Fprintln(&b, "      lan_ports: [lan1, lan2, lan3, lan4]")
	fmt.Fprintln(&b, "      radios:")
	fmt.Fprintln(&b, "        - { name: radio0, band: 2g, htmode: HE20, path: REPLACE_ME }")
	fmt.Fprintln(&b, "        - { name: radio1, band: 5g, htmode: HE80, path: REPLACE_ME }")
	fmt.Fprintln(&b, "      required_packages: [xray-core, kmod-nft-tproxy, xray-geodata]")
	return b.String()
}

func targetOf(b device.BoardInfo) string {
	if b.Release == nil {
		return ""
	}
	return b.Release.Target
}
