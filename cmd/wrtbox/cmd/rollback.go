package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/itslavrov/wrtbox/internal/apply"
)

func newRollbackCmd() *cobra.Command {
	var (
		to   string
		list bool
		r    remoteFlags
	)
	c := &cobra.Command{
		Use:   "rollback",
		Short: "Restore a previous on-router snapshot",
		Long: "rollback replaces the managed files under /etc/{config,xray,...} " +
			"with the contents of a snapshot from /root/wrtbox-backups/<ts> and " +
			"reloads services. With --list it shows the available snapshots " +
			"without making any changes.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cli, _, err := r.dial(ctx)
			if err != nil {
				return err
			}
			defer cli.Close()

			snaps, err := apply.ListBackups(ctx, cli)
			if err != nil {
				return err
			}
			if len(snaps) == 0 {
				return fmt.Errorf("no snapshots found under /root/wrtbox-backups")
			}
			if list {
				for i, s := range snaps {
					marker := ""
					if i == 0 {
						marker = " (latest)"
					}
					fmt.Fprintln(cmd.OutOrStdout(), s+marker)
				}
				return nil
			}

			target := to
			if target == "" {
				// Default: pick the most recent snapshot that isn't
				// the one just created by this same apply run. Without
				// better state we fall back to the latest — the user
				// can pick explicitly with --to.
				target = snaps[0]
			} else {
				ok := false
				for _, s := range snaps {
					if s == target {
						ok = true
						break
					}
				}
				if !ok {
					return fmt.Errorf("snapshot %q not found; available: %s", target, strings.Join(snaps, ", "))
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "restoring snapshot %s\n", target)
			if err := apply.Restore(ctx, cli, "/root/wrtbox-backups/"+target); err != nil {
				return err
			}
			// Reload services so the restored config takes effect.
			reload := "/etc/init.d/network reload && /etc/init.d/firewall reload && " +
				"(/etc/init.d/xray restart 2>/dev/null || /etc/init.d/xray start 2>/dev/null || true) && wifi reload"
			if _, errOut, err := cli.Run(ctx, reload); err != nil {
				return fmt.Errorf("reload: %w — %s", err, string(errOut))
			}
			fmt.Fprintln(cmd.OutOrStdout(), "rollback complete")
			return nil
		},
	}
	c.Flags().StringVar(&to, "to", "", "snapshot timestamp to restore (default: latest)")
	c.Flags().BoolVar(&list, "list", false, "list available snapshots and exit")
	r.bind(c)
	return c
}
