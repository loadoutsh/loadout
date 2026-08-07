package main

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/loadoutsh/loadout/internal/uninstall"
)

func newUninstallCmd() *cobra.Command {
	var (
		yes             bool
		keepConfig      bool
		keepCredentials bool
	)

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove everything loadout has put on this machine",
		Long: `List every file, managed binary and keychain entry loadout created, then
remove them.

Prints the inventory and exits without changing anything unless --yes is
given. Deleting the directories by hand would leave every keychain entry
behind, and unfindable: your config is the only record of what they are
called.

Your shell config is not touched. loadout never added the line that sources
its output, so it does not remove it either; the exact line to delete is
printed instead.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			plan, err := uninstall.Build(uninstall.Options{
				KeepConfig:      keepConfig,
				KeepCredentials: keepCredentials,
			})
			if err != nil {
				return err
			}
			if plan.Empty() {
				cmd.Println("Nothing to remove: loadout does not appear to be installed here.")
				return nil
			}

			printPlan(cmd, plan, keepCredentials)

			if !yes {
				cmd.Println("\nNothing has been changed. Re-run with --yes to remove all of this.")
				return nil
			}

			// Credentials are confirmed on their own. A file can be
			// regenerated; a secret cannot.
			deleteCreds := false
			if len(plan.Credentials) > 0 {
				deleteCreds = confirmCredentials(cmd, plan)
			}

			res := plan.Execute(deleteCreds)

			cmd.Printf("\nRemoved %d item(s).\n", len(res.Removed))
			for _, e := range res.Failed {
				cmd.PrintErrf("  failed: %v\n", e)
			}

			if !deleteCreds && len(plan.Credentials) > 0 {
				cmd.Printf("\nKept %d keychain entr(y/ies). Remove them later with:\n", len(plan.Credentials))
				cmd.Println("  loadout cred remove <credential> <entry>")
				cmd.Println("You will need loadout on PATH to do that, so do it before removing the binary.")
			}

			printManualSteps(cmd, plan)

			if len(res.Failed) > 0 {
				return errSilent
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "actually remove things, rather than only listing them")
	cmd.Flags().BoolVar(&keepConfig, "keep-config", false, "keep ~/.config/loadout, so a reinstall picks up where you left off")
	cmd.Flags().BoolVar(&keepCredentials, "keep-credentials", false, "leave the keychain untouched and do not ask")
	return cmd
}

func printPlan(cmd *cobra.Command, p *uninstall.Plan, keepCredentials bool) {
	cmd.Println("This would remove:")

	if len(p.Paths) > 0 {
		cmd.Println("\nFiles")
		for _, i := range p.Paths {
			cmd.Printf("  %-52s %s\n", i.Name, humanSize(i.Size))
			if i.Note != "" {
				cmd.Printf("  %-52s %s\n", "", i.Note)
			}
		}
	}

	if len(p.Binaries) > 0 {
		cmd.Println("\nExecutables that would leave your PATH")
		for _, i := range p.Binaries {
			cmd.Printf("  %-20s %s\n", i.Name, humanSize(i.Size))
		}
	}

	switch {
	case keepCredentials:
		cmd.Println("\nKeychain\n  untouched (--keep-credentials)")
	case len(p.Credentials) > 0:
		cmd.Printf("\nKeychain entries in %s\n", p.Keyring)
		for _, i := range p.Credentials {
			line := "  " + i.Name
			if i.Note != "" {
				line += "  (" + i.Note + ")"
			}
			cmd.Println(line)
		}
		if !p.CredentialsComplete {
			cmd.Println("\n  This backend cannot be enumerated, so the list comes from your")
			cmd.Println("  config alone. Entries the config has forgotten will not appear.")
		}
	}

	if p.TotalSize() > 0 {
		cmd.Printf("\nTotal on disk: %s\n", humanSize(p.TotalSize()))
	}
}

// confirmCredentials asks separately, and requires the word rather than a
// keystroke, because this is the only irreversible part.
func confirmCredentials(cmd *cobra.Command, p *uninstall.Plan) bool {
	cmd.Printf("\n%d keychain entr(y/ies) will be deleted. If a value is not stored\n", len(p.Credentials))
	cmd.Println("anywhere else, it is gone.")
	cmd.Print("\nType 'delete' to remove them, or anything else to keep them: ")

	sc := bufio.NewScanner(cmd.InOrStdin())
	if !sc.Scan() {
		return false
	}
	return strings.TrimSpace(sc.Text()) == "delete"
}

func printManualSteps(cmd *cobra.Command, p *uninstall.Plan) {
	var steps []string

	if p.ShellRC != "" && p.ShellLine != "" {
		steps = append(steps, fmt.Sprintf("Remove this line from %s:\n     %s", p.ShellRC, p.ShellLine))
	} else {
		steps = append(steps, "Remove the line sourcing loadout's output from your shell config,\n     if you added one.")
	}
	if p.SelfHint != "" {
		steps = append(steps, "Remove the binary:\n     "+p.SelfHint)
	}

	cmd.Println("\nTwo things left, which loadout will not do for you:")
	for i, s := range steps {
		cmd.Printf("  %d. %s\n", i+1, s)
	}
	cmd.Println("\nA new shell will error until that first line is gone.")
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}
