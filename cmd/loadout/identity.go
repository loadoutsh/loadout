package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/loadoutsh/loadout/internal/config"
)

func newIdentityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "identity",
		Short: "Manage which git author, SSH key and credentials are active",
	}
	cmd.AddCommand(newIdentityListCmd(), newIdentityActivateCmd())
	return cmd
}

func newIdentityListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show the configured identities",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if len(cfg.Identities) == 0 {
				cmd.Println("No identities configured. Add them under 'identities:' in your config.")
				return nil
			}
			for _, id := range cfg.Identities {
				marker := " "
				if id.Default {
					marker = "*"
				}
				cmd.Printf("%s %-10s %s <%s>\n", marker, id.ID, id.GitUser, id.Email)
				if id.SSHKey != "" {
					cmd.Printf("  %-10s key: %s\n", "", id.SSHKey)
				}
				for cred, entry := range id.Activates {
					cmd.Printf("  %-10s %s -> %s\n", "", cred, entry)
				}
			}
			cmd.Println("\n* marks the default, used by 's' with no argument.")
			return nil
		},
	}
}

// newIdentityActivateCmd switches the credential entries an identity nominates.
//
// The git config and ssh-add parts stay in the generated shell function, since
// ssh-add needs the calling shell's agent and git config is trivial. This
// command handles only the part that needs to write config: which credential
// entry is active.
func newIdentityActivateCmd() *cobra.Command {
	var export bool

	cmd := &cobra.Command{
		Use:   "activate <id>",
		Short: "Switch this identity's credential entries",
		Long: `Make the credential entries an identity nominates the active ones.

With --export, also print the export statements for the new values, so the
shell function can apply them in one step:

  eval "$(loadout identity activate wc --export)"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			var target *config.Identity
			for i := range cfg.Identities {
				if cfg.Identities[i].ID == args[0] {
					target = &cfg.Identities[i]
					break
				}
			}
			if target == nil {
				return fmt.Errorf("no identity %q", args[0])
			}

			changed := false
			for cred, entry := range target.Activates {
				c, ok := cfg.Credentials[cred]
				if !ok {
					cmd.PrintErrf("# loadout: identity %q nominates unknown credential %q\n", target.ID, cred)
					continue
				}
				if c.Active != entry {
					c.Active = entry
					cfg.Credentials[cred] = c
					changed = true
				}
			}
			if changed {
				if err := cfg.Save(); err != nil {
					return err
				}
			}

			if export {
				// Reuse the env command so there is one implementation of
				// reading the keychain and quoting values.
				env := newEnvCmd()
				env.SetOut(cmd.OutOrStdout())
				env.SetErr(cmd.ErrOrStderr())
				env.SetArgs([]string{"--quiet"})
				return env.Execute()
			}
			if changed {
				cmd.Printf("activated credentials for %s\n", target.ID)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&export, "export", false, "also print export statements for the new values")
	return cmd
}
