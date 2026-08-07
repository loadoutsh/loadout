package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/loadoutsh/loadout/internal/config"
	"github.com/loadoutsh/loadout/internal/sync"
)

func newSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Keep your configuration in a git repository across machines",
		Long: `Track ~/.config/loadout in git so the same configuration follows you
between machines.

Only config.yaml travels. init.zsh is generated on each machine and local.yaml
exists precisely to hold what must not travel. Credentials are never involved:
they live in your OS keychain, which is deliberately not exportable, so a
synced config carries entry names and nothing else.

Use a private repository. The config names your hosts, project paths and
identities even though it holds no secrets.`,
	}
	cmd.AddCommand(
		newSyncInitCmd(),
		newSyncStatusCmd(),
		newSyncPushCmd(),
		newSyncPullCmd(),
		newSyncRemoteCmd(),
	)
	return cmd
}

func repo() (*sync.Repo, error) {
	dir, err := config.Dir()
	if err != nil {
		return nil, err
	}
	return sync.New(dir), nil
}

func newSyncInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init [remote-url]",
		Short: "Turn the config directory into a git repository",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := repo()
			if err != nil {
				return err
			}
			remote := ""
			if len(args) > 0 {
				remote = args[0]
			}
			if err := r.Init(remote); err != nil {
				return err
			}
			cmd.Printf("initialised %s\n", r.Dir)
			cmd.Println("ignored: init.* (generated) and local.yaml (machine-specific)")
			if remote == "" {
				cmd.Println("\nno remote yet: 'loadout sync remote <url>'")
			} else {
				cmd.Printf("remote: %s\n", remote)
			}
			cmd.Println("then: loadout sync push")
			return nil
		},
	}
}

func newSyncRemoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remote [url]",
		Short: "Show or set the remote",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := repo()
			if err != nil {
				return err
			}
			if len(args) == 0 {
				if url := r.Remote(); url != "" {
					cmd.Println(url)
				} else {
					cmd.Println("no remote configured")
				}
				return nil
			}
			if err := r.SetRemote(args[0]); err != nil {
				return err
			}
			cmd.Printf("remote set to %s\n", args[0])
			return nil
		},
	}
}

func newSyncStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show what would be synced",
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := repo()
			if err != nil {
				return err
			}
			st, err := r.Status()
			if err != nil {
				return err
			}

			cmd.Printf("directory: %s\n", r.Dir)
			cmd.Printf("branch:    %s\n", st.Branch)
			if st.Remote == "" {
				cmd.Println("remote:    none")
			} else {
				cmd.Printf("remote:    %s\n", st.Remote)
			}

			files, _ := r.TrackedFiles()
			cmd.Printf("\ntracked (%d):\n", len(files))
			for _, f := range files {
				cmd.Printf("  %s\n", f)
			}

			if st.Dirty {
				cmd.Printf("\nuncommitted (%d):\n", len(st.Changes))
				for _, c := range st.Changes {
					cmd.Printf("  %s\n", c)
				}
			} else {
				cmd.Println("\nno uncommitted changes")
			}
			if st.Ahead > 0 || st.Behind > 0 {
				cmd.Printf("\n%d ahead, %d behind the remote\n", st.Ahead, st.Behind)
			}
			return nil
		},
	}
}

func newSyncPushCmd() *cobra.Command {
	var message string

	cmd := &cobra.Command{
		Use:   "push",
		Short: "Commit and push your configuration",
		Long: `Stage, scan, commit and push.

The scan runs before the commit, not before the push, so a credential that
slipped into the config never enters the history at all. Once committed,
removing it needs a rewrite.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := repo()
			if err != nil {
				return err
			}
			pushed, err := r.Push(message)
			if err != nil {
				return err
			}
			if pushed {
				cmd.Println("pushed")
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "commit message")
	return cmd
}

func newSyncPullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull",
		Short: "Pull configuration from the remote",
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := repo()
			if err != nil {
				return err
			}
			out, err := r.Pull()
			if err != nil {
				return err
			}
			if out != "" {
				cmd.Println(out)
			}
			// The generated file is not tracked, so a pull changes config
			// without changing the shell until apply runs.
			cmd.Println("\nrun 'loadout apply' to regenerate your shell file")
			return nil
		},
	}
}

// scanConfigCmd exposes the scanner on its own, so a user can check before
// committing anywhere else.
func newScanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scan",
		Short: "Check your config for anything that looks like a live credential",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.Path()
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			findings := sync.Scan(string(data))
			if len(findings) == 0 {
				cmd.Printf("%s: nothing that looks like a credential\n", path)
				return nil
			}
			cmd.Printf("%s:\n", path)
			for _, f := range findings {
				cmd.Printf("  %s\n", f)
			}
			return fmt.Errorf("%d suspected credential(s) in your config", len(findings))
		},
	}
}
