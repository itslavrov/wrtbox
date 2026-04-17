package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/itslavrov/wrtbox/internal/apply"
	"github.com/itslavrov/wrtbox/internal/config"
	"github.com/itslavrov/wrtbox/internal/diff"
)

func newApplyCmd() *cobra.Command {
	var (
		cfgPath    string
		dryRun     bool
		backupKeep int
		r          remoteFlags
	)
	c := &cobra.Command{
		Use:   "apply",
		Short: "Render and push the config to the target router over SSH",
		Long: "apply renders wrtbox.yaml locally, uploads the result to " +
			"/root/wrtbox-staging/<ts> on the router, validates it (uci + xray -test), " +
			"snapshots the current state under /root/wrtbox-backups/<ts>, swaps the " +
			"managed files into place, reloads services, and post-checks VPN " +
			"reachability. On any failure past the snapshot step it automatically rolls back.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			if dryRun {
				_, err := apply.Run(ctx, nil, cfg, apply.Options{
					DryRun:     true,
					Writer:     cmd.OutOrStdout(),
					BackupKeep: backupKeep,
				})
				return err
			}

			cli, rt, err := r.dial(ctx)
			if err != nil {
				return err
			}
			defer cli.Close()
			fmt.Fprintf(cmd.OutOrStdout(), "connected to %s@%s:%d\n", rt.User, rt.Host, rt.Port)

			res, err := apply.Run(ctx, cli, cfg, apply.Options{
				Writer:     cmd.OutOrStdout(),
				BackupKeep: backupKeep,
			})
			if res != nil && res.RolledBack {
				return fmt.Errorf("apply rolled back: %w", err)
			}
			return err
		},
	}
	c.Flags().StringVarP(&cfgPath, "config", "c", "wrtbox.yaml", "path to wrtbox config")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "print what would happen without connecting")
	c.Flags().IntVar(&backupKeep, "backup-keep", 5, "number of on-router snapshots to retain")
	r.bind(c)
	return c
}

func newDiffCmd() *cobra.Command {
	var (
		cfgPath string
		r       remoteFlags
	)
	c := &cobra.Command{
		Use:   "diff",
		Short: "Show the diff between rendered desired state and the router's current state",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			cli, _, err := r.dial(ctx)
			if err != nil {
				return err
			}
			defer cli.Close()
			sum, err := diff.Run(ctx, cli, cfg, diff.Options{Writer: cmd.OutOrStdout()})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d changed, %d added\n", len(sum.Changed), len(sum.Added))
			return nil
		},
	}
	c.Flags().StringVarP(&cfgPath, "config", "c", "wrtbox.yaml", "path to wrtbox config")
	r.bind(c)
	return c
}
