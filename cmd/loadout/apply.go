package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/wesscoby/loadout/internal/catalog"
	"github.com/wesscoby/loadout/internal/config"
	"github.com/wesscoby/loadout/internal/generate"
)

func newApplyCmd() *cobra.Command {
	var (
		shell  string
		goos   string
		stdout bool
		out    string
	)

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Render your config into the shell file",
		Long: `Render the catalog against your configuration and write the shell file
that your shell sources at startup.

Nothing outside ~/.config/loadout is touched.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cat, err := catalog.Load()
			if err != nil {
				return err
			}

			binDir, err := managedBinDir()
			if err != nil {
				return err
			}

			g, err := generate.New(cat, cfg, generate.Options{
				Shell:  shell,
				GOOS:   goos,
				BinDir: binDir,
			})
			if err != nil {
				return err
			}
			rendered, err := g.Render()
			if err != nil {
				return err
			}

			if stdout {
				_, err := cmd.OutOrStdout().Write(rendered)
				return err
			}

			path := out
			if path == "" {
				path, err = generate.OutputPath(shell)
				if err != nil {
					return err
				}
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, rendered, 0o644); err != nil {
				return err
			}

			cmd.Printf("wrote %s\n", path)
			cmd.Printf("add this to your shell config if it is not there yet:\n\n  source %s\n", path)
			return nil
		},
	}

	cmd.Flags().StringVar(&shell, "shell", "zsh", "shell dialect to generate")
	cmd.Flags().StringVar(&goos, "platform", runtime.GOOS, "target platform (linux, darwin)")
	cmd.Flags().BoolVar(&stdout, "stdout", false, "write to stdout instead of the config directory")
	cmd.Flags().StringVar(&out, "out", "", "write to this path instead of the default")
	return cmd
}

// managedBinDir is ~/.local/share/loadout/bin, honouring XDG_DATA_HOME.
func managedBinDir() (string, error) {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "loadout", "bin"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "loadout", "bin"), nil
}
