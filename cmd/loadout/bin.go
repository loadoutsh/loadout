package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/loadoutsh/loadout/internal/binaries"
	"github.com/loadoutsh/loadout/internal/catalog"
	"github.com/loadoutsh/loadout/internal/config"
)

func newBinCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bin",
		Short: "Manage the executables loadout keeps on your PATH",
		Long: `Third-party binaries are declared in your config and fetched per platform,
rather than committed anywhere. Scripts are either shipped with loadout or
symlinked from wherever you keep them.

Everything lands in one managed directory, appended to PATH so nothing here
can shadow a system tool.`,
	}
	cmd.AddCommand(
		newBinListCmd(),
		newBinInstallCmd(),
		newBinRemoveCmd(),
	)
	return cmd
}

// installer builds an Installer pointed at the managed directories.
func installer() (*binaries.Installer, error) {
	binDir, err := managedBinDir()
	if err != nil {
		return nil, err
	}
	return binaries.NewInstaller(binDir, filepath.Dir(binDir)), nil
}

func builtinLookup() (binaries.BuiltinLookup, error) {
	cat, err := catalog.Load()
	if err != nil {
		return nil, err
	}
	return func(name string) ([]byte, error) {
		s, err := cat.Script(name)
		if err != nil {
			return nil, err
		}
		return s.Body, nil
	}, nil
}

func newBinListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show what is installed, what is pending, and where it came from",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			inst, err := installer()
			if err != nil {
				return err
			}
			statuses, err := inst.Status(cfg.Binaries)
			if err != nil {
				return err
			}

			cmd.Printf("bin directory: %s\n", inst.BinDir)
			cmd.Printf("platform:      %s\n", binaries.Current())

			if len(statuses) == 0 {
				cmd.Println("\nNo binaries declared. Add them under 'binaries:' in your config.")
			} else {
				cmd.Println("\nBinaries")
				for _, s := range statuses {
					cmd.Printf("  %-14s %s\n", s.Declared.Name, binStatusLine(s))
					if s.Installed && s.Record.URL != "" {
						cmd.Printf("  %-14s   from %s\n", "", s.Record.URL)
						verified := "not verified (no checksum declared)"
						if s.Record.Verified {
							verified = "checksum verified"
						}
						cmd.Printf("  %-14s   %s\n", "", verified)
					}
				}
			}

			if len(cfg.Scripts) > 0 {
				cmd.Println("\nScripts")
				for _, s := range cfg.Scripts {
					marker := "missing"
					target := filepath.Join(inst.BinDir, s.Name)
					if _, err := os.Lstat(target); err == nil {
						marker = "installed"
					}
					cmd.Printf("  %-14s %-10s (%s)\n", s.Name, marker, s.Source)
				}
			}
			return nil
		},
	}
}

func binStatusLine(s binaries.Status) string {
	switch {
	case s.Orphaned:
		return fmt.Sprintf("%s  installed but no longer declared", s.Record.Version)
	case !s.Installed:
		return fmt.Sprintf("%s  not installed (run 'loadout bin install')", s.Declared.Version)
	case s.Stale:
		return fmt.Sprintf("%s  installed, config declares %s", s.Record.Version, s.Declared.Version)
	default:
		return fmt.Sprintf("%s  installed", s.Record.Version)
	}
}

func newBinInstallCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "install [name...]",
		Short: "Install declared binaries and scripts",
		Long: `Fetch and install everything declared in your config, or just the named ones.

Already-current binaries are skipped unless --force is given. Downloads are
HTTPS only, and a declared checksum is verified before anything is installed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			inst, err := installer()
			if err != nil {
				return err
			}

			wanted := map[string]bool{}
			for _, a := range args {
				wanted[a] = true
			}

			statuses, err := inst.Status(cfg.Binaries)
			if err != nil {
				return err
			}

			plat := binaries.Current()
			installed, skipped, failed := 0, 0, 0

			for _, s := range statuses {
				if s.Orphaned {
					continue
				}
				if len(wanted) > 0 && !wanted[s.Declared.Name] {
					continue
				}
				if s.Installed && !s.Stale && !force {
					cmd.Printf("  %-14s already current (%s)\n", s.Declared.Name, s.Record.Version)
					skipped++
					continue
				}

				cmd.Printf("  %-14s fetching %s...\n", s.Declared.Name, s.Declared.Version)
				rec, err := inst.Install(s.Declared, plat)
				if err != nil {
					cmd.Printf("  %-14s FAILED: %v\n", s.Declared.Name, err)
					failed++
					continue
				}
				note := ""
				if !rec.Verified {
					note = "  (no checksum declared, not verified)"
				}
				cmd.Printf("  %-14s installed %s%s\n", s.Declared.Name, rec.Version, note)
				installed++
			}

			// Scripts are cheap and idempotent, so they are always brought up
			// to date rather than needing their own command.
			if len(cfg.Scripts) > 0 && len(wanted) == 0 {
				lookup, err := builtinLookup()
				if err != nil {
					return err
				}
				for _, r := range inst.InstallScripts(cfg.Scripts, lookup) {
					if r.Err != nil {
						cmd.Printf("  %-14s FAILED: %v\n", r.Name, r.Err)
						failed++
						continue
					}
					if r.Action != "unchanged" {
						cmd.Printf("  %-14s %s (%s)\n", r.Name, r.Action, r.Source)
					}
				}
			}

			cmd.Printf("\n%d installed, %d already current, %d failed\n", installed, skipped, failed)
			if failed > 0 {
				return errSilent
			}
			if installed > 0 {
				cmd.Printf("\n%s is on PATH once you reload your shell\n", inst.BinDir)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "reinstall even when already current")
	return cmd
}

func newBinRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a managed binary",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inst, err := installer()
			if err != nil {
				return err
			}
			if err := inst.Remove(args[0]); err != nil {
				return err
			}
			cmd.Printf("removed %s\n", args[0])

			// Leaving it declared means the next install brings it straight
			// back, which would look like the removal did not work.
			cfg, err := config.Load()
			if err == nil {
				for _, b := range cfg.Binaries {
					if b.Name == args[0] {
						cmd.Printf("note: %q is still declared in your config; "+
							"remove it there too or 'loadout bin install' will restore it\n", args[0])
					}
				}
			}
			return nil
		},
	}
}

// scriptNames is used by apply to report what it installed.
func scriptNames(rs []binaries.ScriptResult) string {
	var names []string
	for _, r := range rs {
		if r.Err == nil && r.Action != "unchanged" {
			names = append(names, r.Name)
		}
	}
	return strings.Join(names, ", ")
}
