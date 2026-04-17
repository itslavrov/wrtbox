package cmd

import (
	_ "embed"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

//go:embed assets/wrtbox.example.yaml
var initTemplate []byte

func newInitCmd() *cobra.Command {
	var (
		outPath string
		force   bool
	)
	c := &cobra.Command{
		Use:   "init",
		Short: "Write a starter wrtbox.yaml with inline comments",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := os.Stat(outPath); err == nil && !force {
				return fmt.Errorf("%s already exists (use --force to overwrite)", outPath)
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err := os.WriteFile(outPath, initTemplate, 0o600); err != nil {
				return fmt.Errorf("write %s: %w", outPath, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", outPath)
			return nil
		},
	}
	c.Flags().StringVarP(&outPath, "out", "o", "wrtbox.yaml", "where to write the starter file")
	c.Flags().BoolVar(&force, "force", false, "overwrite an existing file")
	return c
}
