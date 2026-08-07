package main

import (
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

	root.AddCommand(
		newApplyCmd(),
		newInitCmd(),
		newCatalogCmd(),
		newDescribeCmd(),
		newDoctorCmd(),
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
