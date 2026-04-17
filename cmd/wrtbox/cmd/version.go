package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/itslavrov/wrtbox/internal/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build metadata",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "wrtbox %s (commit %s, built %s)\n",
				version.Version, version.Commit, version.BuildDate)
			return nil
		},
	}
}
