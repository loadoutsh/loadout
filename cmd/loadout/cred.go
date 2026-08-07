package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/wesscoby/loadout/internal/config"
	"github.com/wesscoby/loadout/internal/credentials"
)

func newCredCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cred",
		Short: "Manage secrets stored in your OS keychain",
		Long: `A credential is a named secret with one or more entries, one of which is
active. Switching the active entry is how you swap between, say, a personal
and a work npm token without editing anything.

Values live in the OS keychain and are never written to a file loadout
controls. Your config records only the entry names and which one is active.`,
	}
	cmd.AddCommand(
		newCredListCmd(),
		newCredSetCmd(),
		newCredUseCmd(),
		newCredRemoveCmd(),
	)
	return cmd
}

func newCredListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show declared credentials, which entry is active, and which have values",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if len(cfg.Credentials) == 0 {
				cmd.Println("No credentials declared. Add them under 'credentials:' in your config.")
				return nil
			}

			ring, ringErr := credentials.Open()
			if ringErr != nil {
				cmd.PrintErrf("warning: %v\n\n", ringErr)
			}

			names := make([]string, 0, len(cfg.Credentials))
			for n := range cfg.Credentials {
				names = append(names, n)
			}
			sort.Strings(names)

			if ringErr == nil {
				cmd.Printf("keychain: %s\n\n", ring.Name())
			}
			for _, name := range names {
				c := cfg.Credentials[name]
				cmd.Printf("%s\n", name)
				for _, entry := range c.Entries {
					marker := "  "
					if entry == c.Active {
						marker = "* "
					}
					state := "no value stored"
					if ringErr == nil {
						if _, err := ring.Get(credentials.Key(name, entry)); err == nil {
							state = "value stored"
						}
					} else {
						state = "unknown"
					}
					cmd.Printf("  %s%-14s %s\n", marker, entry, state)
				}
			}
			cmd.Println("\n* marks the active entry. Set values with 'loadout cred set <name> <entry>'.")
			return nil
		},
	}
}

func newCredSetCmd() *cobra.Command {
	var stdin bool

	cmd := &cobra.Command{
		Use:   "set <credential> <entry>",
		Short: "Store a value in the keychain",
		Long: `Store a secret. By default the value is read from the terminal without
echoing it. Use --stdin to pipe it instead, which avoids the value reaching
your shell history.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, entry := args[0], args[1]

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			// Refuse to store something the config does not declare, so a typo
			// cannot leave an orphaned secret in the keychain.
			c, ok := cfg.Credentials[name]
			if !ok {
				return fmt.Errorf("credential %q is not declared in your config", name)
			}
			if !containsStr(c.Entries, entry) {
				return fmt.Errorf("credential %q has no entry %q (declared: %s)",
					name, entry, strings.Join(c.Entries, ", "))
			}

			ring, err := credentials.Open()
			if err != nil {
				return err
			}

			value, err := readSecret(cmd, stdin)
			if err != nil {
				return err
			}
			if value == "" {
				return errors.New("refusing to store an empty value")
			}

			if err := ring.Set(credentials.Key(name, entry), value); err != nil {
				return err
			}
			cmd.Printf("stored %s/%s in %s\n", name, entry, ring.Name())
			if c.Active != entry {
				cmd.Printf("note: %q is active for %s; run 'loadout cred use %s %s' to switch\n",
					c.Active, name, name, entry)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&stdin, "stdin", false, "read the value from stdin instead of prompting")
	return cmd
}

// readSecret gets a value without it appearing on screen or in shell history.
func readSecret(cmd *cobra.Command, fromStdin bool) (string, error) {
	if fromStdin {
		data, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if err != nil && data == "" {
			return "", err
		}
		return strings.TrimRight(data, "\r\n"), nil
	}

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("not a terminal: use --stdin to pipe the value")
	}
	cmd.Print("value (not echoed): ")
	b, err := term.ReadPassword(fd)
	cmd.Println()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func newCredUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <credential> <entry>",
		Short: "Make an entry the active one",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, entry := args[0], args[1]

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			c, ok := cfg.Credentials[name]
			if !ok {
				return fmt.Errorf("credential %q is not declared in your config", name)
			}
			if !containsStr(c.Entries, entry) {
				return fmt.Errorf("credential %q has no entry %q (declared: %s)",
					name, entry, strings.Join(c.Entries, ", "))
			}

			c.Active = entry
			cfg.Credentials[name] = c
			if err := cfg.Save(); err != nil {
				return err
			}

			cmd.Printf("%s is now %s\n", name, entry)
			cmd.Println("run 'eval \"$(loadout env)\"' or reload your shell to pick it up")
			return nil
		},
	}
}

func newCredRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <credential> <entry>",
		Short: "Delete a stored value from the keychain",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ring, err := credentials.Open()
			if err != nil {
				return err
			}
			if err := ring.Delete(credentials.Key(args[0], args[1])); err != nil {
				return err
			}
			cmd.Printf("removed %s/%s from %s\n", args[0], args[1], ring.Name())
			cmd.Println("note: the entry is still declared in your config; remove it there too if you are done with it")
			return nil
		},
	}
}

func containsStr(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
