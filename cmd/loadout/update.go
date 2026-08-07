package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/loadoutsh/loadout/internal/selfupdate"
)

func newUpdateCmd() *cobra.Command {
	var (
		checkOnly bool
		force     bool
		baseURL   string
		release   string
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update loadout to the latest release",
		Long: `Check for a newer release and install it.

Only replaces the binary when loadout owns the file. If Homebrew or the Go
toolchain installed it, that tool owns it, and replacing the file behind its
back leaves it reporting a version it never installed. In those cases this
prints the right command instead.

The download is HTTPS on every hop, its checksum is verified, and the
replacement is a rename, so an interrupted update cannot leave a broken
binary on your PATH.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			method, exe := selfupdate.Detect()

			client := selfupdate.NewClient()
			if baseURL != "" {
				client.BaseURL = baseURL
				client.Version = release
			}

			rel, err := client.Latest()
			if err != nil {
				return err
			}

			cmd.Printf("installed: %s\n", Version)
			cmd.Printf("latest:    %s\n", rel.Version)
			cmd.Printf("installed via %s\n", method)

			switch selfupdate.Compare(Version, rel.Version) {
			case selfupdate.Same:
				cmd.Println("\nAlready up to date.")
				return nil
			case selfupdate.Newer:
				cmd.Println("\nYour build is ahead of the latest release. Nothing to do.")
				return nil
			case selfupdate.Unknown:
				// Say which side is not a release tag. A development build and
				// an unresolvable release are different problems, and reporting
				// them the same way sends people looking in the wrong place.
				curOK, latestOK := selfupdate.Comparable(Version, rel.Version)
				switch {
				case !latestOK && !curOK:
					cmd.Printf("\nNeither %q nor the published version %q is a release tag.\n", Version, rel.Version)
				case !latestOK:
					cmd.Printf("\nCould not read a release version from the server (got %q).\n", rel.Version)
				default:
					cmd.Printf("\n%q is a development build, so there is nothing to compare it against.\n", Version)
				}
				if !force {
					cmd.Println("Re-run with --force to install the latest release anyway.")
					return nil
				}
			case selfupdate.Older:
				cmd.Printf("\nAn update is available: %s -> %s\n", Version, rel.Version)
			}

			if !method.SelfUpdatable() {
				cmd.Printf("\n%s owns this binary, so loadout will not replace it.\nRun:\n\n  %s\n",
					method, method.UpdateCommand())
				return nil
			}

			if checkOnly {
				cmd.Println("\nRun 'loadout update' without --check to install it.")
				return nil
			}

			cmd.Printf("\nreplacing %s\n", exe)
			if err := client.Apply(rel, exe); err != nil {
				return err
			}
			cmd.Printf("updated to %s\n", rel.Version)
			cmd.Println("\nYour configuration is untouched. Run 'loadout apply' if the")
			cmd.Println("release notes mention catalog changes.")
			return nil
		},
	}

	cmd.Flags().BoolVar(&checkOnly, "check", false, "report whether an update exists, install nothing")
	cmd.Flags().BoolVar(&force, "force", false, "install the latest release even when versions cannot be compared")
	cmd.Flags().StringVar(&baseURL, "base-url", os.Getenv("LOADOUT_BASE_URL"),
		"fetch releases from here instead of GitHub, for a mirror")
	cmd.Flags().StringVar(&release, "release", os.Getenv("LOADOUT_VERSION"),
		"which release to fetch; required with --base-url")
	return cmd
}
