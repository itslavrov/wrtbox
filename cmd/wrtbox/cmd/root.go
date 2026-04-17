// Package cmd wires wrtbox subcommands under a cobra root.
package cmd

import (
	"github.com/spf13/cobra"

	"github.com/itslavrov/wrtbox/internal/version"
)

// NewRootCmd returns the top-level cobra command.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "wrtbox",
		Short:         "Declarative OpenWrt split-routing toolkit",
		Long:          "wrtbox turns a single YAML file into a reproducible OpenWrt + Xray router setup.",
		Version:       versionString(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetVersionTemplate("wrtbox {{.Version}}\n")
	root.AddCommand(
		newInitCmd(),
		newValidateCmd(),
		newRenderCmd(),
		newApplyCmd(),
		newDiffCmd(),
		newRollbackCmd(),
		newDetectCmd(),
		newVersionCmd(),
	)
	return root
}

func versionString() string {
	return version.Version + " (commit " + version.Commit + ", built " + version.BuildDate + ")"
}
