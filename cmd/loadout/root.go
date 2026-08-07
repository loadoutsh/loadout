package main

import (
	"os"

	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "loadout",
		Short: "A curated shell environment you configure rather than edit",
		Long: `loadout renders a curated set of aliases and functions against your own
configuration, producing one shell file that your shell sources at startup.

Edit ~/.config/loadout/config.yaml, run 'loadout apply', reload your shell.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Cobra's Print helpers fall back to stderr when no out writer is set.
	// That silently broke `eval "$(loadout env)"`, which captures stdout only,
	// so the exports went nowhere. Set both explicitly.
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)

	root.AddCommand(
		newApplyCmd(),
		newInitCmd(),
		newCatalogCmd(),
		newDescribeCmd(),
		newDoctorCmd(),
		newBinCmd(),
		newCredCmd(),
		newEnvCmd(),
		newIdentityCmd(),
		newUICmd(),
		newSyncCmd(),
		newScanCmd(),
		newUninstallCmd(),
		newVersionCmd(),
	)
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println(Version)
			return nil
		},
	}
}
