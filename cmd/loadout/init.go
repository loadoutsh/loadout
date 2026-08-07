package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wesscoby/loadout/internal/catalog"
	"github.com/wesscoby/loadout/internal/config"
	"github.com/wesscoby/loadout/internal/generate"
)

func newInitCmd() *cobra.Command {
	var (
		force    bool
		defaults bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create your configuration",
		Long: `Ask a few questions and write ~/.config/loadout/config.yaml.

Nothing is guessed silently: every detected value is offered as a default you
can accept or replace. Use --defaults to skip the questions entirely.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.Path()
			if err != nil {
				return err
			}
			if _, err := os.Stat(path); err == nil && !force {
				return fmt.Errorf("%s already exists (use --force to overwrite)", path)
			}

			cfg := config.Default()
			if !defaults {
				if err := runWizard(cmd, cfg); err != nil {
					return err
				}
			}
			if err := cfg.Save(); err != nil {
				return err
			}

			cmd.Printf("\nwrote %s\n", path)

			genPath, err := generate.OutputPath("zsh")
			if err == nil {
				cmd.Println("\nnext:")
				cmd.Println("  loadout apply            render your shell file")
				cmd.Println("  loadout doctor           check everything works here")
				cmd.Printf("\nthen add this line to ~/.zshrc:\n\n  source %s\n", genPath)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing config")
	cmd.Flags().BoolVar(&defaults, "defaults", false, "accept all defaults without asking")
	return cmd
}

// prompter reads answers, falling back to the offered default on empty input.
type prompter struct {
	in  *bufio.Scanner
	cmd *cobra.Command
}

func newPrompter(cmd *cobra.Command) *prompter {
	return &prompter{in: bufio.NewScanner(cmd.InOrStdin()), cmd: cmd}
}

func (p *prompter) ask(question, def string) string {
	if def != "" {
		p.cmd.Printf("%s [%s]: ", question, def)
	} else {
		p.cmd.Printf("%s: ", question)
	}
	if !p.in.Scan() {
		return def
	}
	answer := strings.TrimSpace(p.in.Text())
	if answer == "" {
		return def
	}
	return answer
}

func (p *prompter) confirm(question string, def bool) bool {
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	p.cmd.Printf("%s [%s]: ", question, hint)
	if !p.in.Scan() {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(p.in.Text())) {
	case "":
		return def
	case "y", "yes":
		return true
	default:
		return false
	}
}

func runWizard(cmd *cobra.Command, cfg *config.Config) error {
	p := newPrompter(cmd)

	cmd.Println("Setting up loadout. Press enter to accept each default.")

	// --- paths ---
	cmd.Println("\nPaths")
	cfg.Paths.ProjectRoot = p.ask("  where your projects live", cfg.Paths.ProjectRoot)
	cfg.Paths.VenvRoot = p.ask("  where Python virtualenvs live", cfg.Paths.VenvRoot)

	// --- editor ---
	cmd.Println("\nEditor")
	editor := p.ask("  command to open a project", detectEditor())
	if editor != "" {
		cfg.Editors = []config.Editor{{Name: filepath.Base(editor), Command: editor}}
	}

	// --- git ---
	cmd.Println("\nGit")
	cfg.Git.MainBranch = p.ask("  name of your main branch", cfg.Git.MainBranch)
	cfg.Git.DevBranch = p.ask("  name of your development branch", cfg.Git.DevBranch)

	// --- identity ---
	cmd.Println("\nIdentity")
	cmd.Println("  An identity is a git author plus the SSH key that goes with it.")
	name := p.ask("  git user name", gitConfigValue("user.name"))
	email := p.ask("  git email", gitConfigValue("user.email"))
	if name != "" || email != "" {
		keys := detectSSHKeys()
		defKey := ""
		if len(keys) > 0 {
			defKey = keys[0]
			shown := keys
			if len(shown) > 6 {
				shown = shown[:6]
			}
			cmd.Printf("  keys in ~/.ssh: %s", strings.Join(shown, " "))
			if len(keys) > len(shown) {
				cmd.Printf(" (+%d more)", len(keys)-len(shown))
			}
			cmd.Println()
		}
		key := p.ask("  ssh key for this identity", defKey)
		cfg.Identities = []config.Identity{{
			ID:      "default",
			GitUser: name,
			Email:   email,
			SSHKey:  key,
			Default: true,
		}}
	}

	// --- packs ---
	cat, err := catalog.Load()
	if err != nil {
		return err
	}
	available := detectPacks(cat)
	if len(available) > 0 {
		cmd.Println("\nPacks")
		cmd.Println("  Optional alias bundles. These look usable on this machine:")
		for _, pk := range available {
			if p.confirm(fmt.Sprintf("    enable %s?", pk), false) {
				cfg.Packs = append(cfg.Packs, pk)
			}
		}
	}

	return nil
}

// detectEditor returns the first editor found on PATH.
func detectEditor() string {
	for _, e := range []string{"code", "cursor", "subl", "nvim", "vim"} {
		if _, err := exec.LookPath(e); err == nil {
			return e
		}
	}
	return ""
}

// gitConfigValue reads an existing global git setting so the wizard can offer
// what the user already uses.
func gitConfigValue(key string) string {
	out, err := exec.Command("git", "config", "--global", "--get", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// conventionalKeys are the names ssh itself tries by default, so if one is
// present it is far more likely to be the user's main key than whatever
// happens to sort first.
var conventionalKeys = []string{"id_ed25519", "id_rsa", "id_ecdsa", "id_dsa"}

// detectSSHKeys lists probable private keys in ~/.ssh, most likely first.
// Public halves, known_hosts variants and ssh's own files are excluded.
func detectSSHKeys() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	dir := filepath.Join(home, ".ssh")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	skip := map[string]bool{
		"config": true, "authorized_keys": true, "environment": true,
		"authorized_keys2": true, "rc": true,
	}

	var keys []string
	for _, e := range entries {
		n := e.Name()
		switch {
		case e.IsDir(), skip[n]:
			continue
		case strings.HasSuffix(n, ".pub"):
			continue
		// Catches known_hosts, known_hosts.old, and prefixed variants such as
		// google_compute_known_hosts.
		case strings.Contains(n, "known_hosts"):
			continue
		}
		keys = append(keys, "~/.ssh/"+n)
	}
	sort.Strings(keys)

	// Float a conventionally named key to the front so it becomes the default.
	for i := len(conventionalKeys) - 1; i >= 0; i-- {
		want := "~/.ssh/" + conventionalKeys[i]
		for j, k := range keys {
			if k == want {
				keys = append([]string{k}, append(keys[:j:j], keys[j+1:]...)...)
				break
			}
		}
	}
	return keys
}

// detectPacks reports packs whose tools are already installed, so the wizard
// only offers what would actually work.
func detectPacks(cat *catalog.Catalog) []string {
	byPack := map[string][]string{}
	for _, a := range cat.Aliases {
		if a.Pack == "" {
			continue
		}
		byPack[a.Pack] = append(byPack[a.Pack], cat.RequiredBinaries(a, hostGOOS())...)
	}

	var usable []string
	for pack, bins := range byPack {
		for _, b := range bins {
			if _, err := exec.LookPath(b); err == nil {
				usable = append(usable, pack)
				break
			}
		}
	}
	sort.Strings(usable)
	return usable
}

func newCatalogCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "catalog",
		Short: "Show what the shipped catalog contains",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cat, err := catalog.Load()
			if err != nil {
				return err
			}
			core, packed := 0, map[string]int{}
			for _, a := range cat.Aliases {
				if a.Pack == "" {
					core++
					continue
				}
				packed[a.Pack]++
			}
			cmd.Printf("aliases:   %d (%d core, %d in packs)\n", len(cat.Aliases), core, len(cat.Aliases)-core)
			cmd.Printf("functions: %d\n", len(cat.Functions))
			cmd.Println("packs:")
			for _, p := range cat.Packs() {
				cmd.Printf("  %-14s %d aliases\n", p, packed[p])
			}
			return nil
		},
	}
}
