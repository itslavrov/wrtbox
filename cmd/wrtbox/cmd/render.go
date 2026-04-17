package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/itslavrov/wrtbox/internal/config"
	"github.com/itslavrov/wrtbox/internal/render"
)

func newRenderCmd() *cobra.Command {
	var (
		cfgPath string
		outDir  string
	)
	c := &cobra.Command{
		Use:   "render",
		Short: "Render router config from wrtbox.yaml into an output tree",
		Long: "Reads wrtbox.yaml, resolves lists, and writes /etc/config, /etc/xray and helper " +
			"scripts under --out. The output is self-contained and ready to be rsync'd to a router.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			files, err := render.Render(cmd.Context(), cfg, render.Options{})
			if err != nil {
				return err
			}
			if err := writeTree(outDir, files); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "rendered %d files under %s\n", len(files), outDir)
			return nil
		},
	}
	c.Flags().StringVarP(&cfgPath, "config", "c", "wrtbox.yaml", "path to wrtbox config")
	c.Flags().StringVarP(&outDir, "out", "o", "", "output directory (required)")
	_ = c.MarkFlagRequired("out")
	c.SetContext(context.Background())
	return c
}

func writeTree(root string, files []render.File) error {
	for _, f := range files {
		full := filepath.Join(root, f.Path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, f.Data, os.FileMode(f.Mode)); err != nil {
			return fmt.Errorf("write %s: %w", full, err)
		}
	}
	return nil
}
