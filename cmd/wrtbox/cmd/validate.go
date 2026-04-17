package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/itslavrov/wrtbox/internal/config"
)

func newValidateCmd() *cobra.Command {
	var cfgPath string
	c := &cobra.Command{
		Use:   "validate",
		Short: "Parse and validate a wrtbox.yaml",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"ok: %s (device=%s, profile=%s, routing.lists=%d, force_via_vpn=%d, force_direct=%d, block=%d)\n",
				cfg.Metadata.Name,
				cfg.Spec.Device.Model,
				cfg.Spec.Routing.Profile,
				len(cfg.Spec.Routing.Lists),
				len(cfg.Spec.Routing.ForceViaVPN),
				len(cfg.Spec.Routing.ForceDirect),
				len(cfg.Spec.Routing.Block),
			)
			return nil
		},
	}
	c.Flags().StringVarP(&cfgPath, "config", "c", "wrtbox.yaml", "path to wrtbox config")
	_ = c.MarkFlagRequired("config")
	return c
}
