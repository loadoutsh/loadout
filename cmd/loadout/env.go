package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wesscoby/loadout/internal/config"
	"github.com/wesscoby/loadout/internal/credentials"
)

func newEnvCmd() *cobra.Command {
	var (
		identity string
		quiet    bool
	)

	cmd := &cobra.Command{
		Use:   "env",
		Short: "Print export lines for the active credentials",
		Long: `Print shell export statements for whichever credential entry is currently
active, reading the values from the keychain.

  eval "$(loadout env)"

Secrets are deliberately not written into the generated shell file, because
that file is plain text on disk. The cost of that choice is this command:
something has to run to fetch them. It is not added to your shell startup
automatically; put the eval above in your shell config only if you want the
tokens present in every shell.

Names are upper-cased, so a credential called npm_token exports NPM_TOKEN.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			// An identity may nominate different entries; --identity renders
			// the environment as it would be after switching to it.
			overrides := map[string]string{}
			if identity != "" {
				found := false
				for _, id := range cfg.Identities {
					if id.ID != identity {
						continue
					}
					found = true
					for cred, entry := range id.Activates {
						overrides[cred] = entry
					}
				}
				if !found {
					return fmt.Errorf("no identity %q", identity)
				}
			}

			if len(cfg.Credentials) == 0 {
				return nil
			}

			ring, err := credentials.Open()
			if err != nil {
				// Emitting nothing is right: this output gets eval'd, and a
				// missing keychain should not break shell startup.
				if !quiet {
					cmd.PrintErrf("# loadout: %v\n", err)
				}
				return nil
			}

			names := make([]string, 0, len(cfg.Credentials))
			for n := range cfg.Credentials {
				names = append(names, n)
			}
			sort.Strings(names)

			var missing []string
			for _, name := range names {
				c := cfg.Credentials[name]
				entry := c.Active
				if o, ok := overrides[name]; ok {
					entry = o
				}
				if entry == "" {
					continue
				}

				value, err := ring.Get(credentials.Key(name, entry))
				if err != nil {
					if errors.Is(err, credentials.ErrNotFound) {
						missing = append(missing, fmt.Sprintf("%s/%s", name, entry))
						continue
					}
					if !quiet {
						cmd.PrintErrf("# loadout: %s: %v\n", name, err)
					}
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "export %s=%s\n", envName(name), shellQuote(value))
			}

			if len(missing) > 0 && !quiet {
				cmd.PrintErrf("# loadout: no value stored for %s (set with 'loadout cred set')\n",
					strings.Join(missing, ", "))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&identity, "identity", "", "render the environment for this identity's entries")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "suppress diagnostics, for use in shell startup")
	return cmd
}

// envName turns a credential name into an environment variable name.
func envName(name string) string {
	return strings.ToUpper(strings.NewReplacer("-", "_", ".", "_", "/", "_").Replace(name))
}

// shellQuote single-quotes a value for safe eval. A secret can contain
// anything at all, so nothing may be left for the shell to interpret.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
