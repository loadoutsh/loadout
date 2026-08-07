package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wesscoby/loadout/internal/catalog"
	"github.com/wesscoby/loadout/internal/config"
	"github.com/wesscoby/loadout/internal/requires"
)

func newDescribeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe [command]",
		Short: "Explain what a command does and what it needs to work",
		Long: `Print what a command is for, how to call it, which configuration shapes it,
and whether its prerequisites are met on this machine.

With no argument, lists everything available.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := catalog.Load()
			if err != nil {
				return err
			}

			// Describe should still work before there is a config, since one
			// reason to run it is to find out what to configure.
			cfg, cfgErr := config.Load()
			if cfgErr != nil {
				cfg = config.Default()
			}

			if len(args) == 0 {
				listAll(cmd, cat)
				return nil
			}
			return describeOne(cmd, cat, cfg, cfgErr == nil, args[0])
		},
	}
	return cmd
}

func listAll(cmd *cobra.Command, cat *catalog.Catalog) {
	fns := make([]catalog.Function, len(cat.Functions))
	copy(fns, cat.Functions)
	sort.Slice(fns, func(i, j int) bool { return fns[i].Name < fns[j].Name })

	cmd.Println("Commands:")
	for _, f := range fns {
		summary := f.Summary
		if summary == "" {
			summary = "(no description)"
		}
		cmd.Printf("  %-14s %s\n", f.Name, summary)
	}
	cmd.Println("\nAliases are listed by 'loadout catalog'.")
	cmd.Println("Describe one command with 'loadout describe <name>'.")
}

func describeOne(cmd *cobra.Command, cat *catalog.Catalog, cfg *config.Config, haveConfig bool, name string) error {
	fn, found := cat.Function(name)
	resolvedName := name

	// A name may belong to a family generated from config rather than to a
	// template. `wt` is not in the catalog; it comes from clone_sources.
	if !found {
		if fam, famName, okFam := familyFor(cat, cfg, name); okFam {
			fn, found, resolvedName = fam, true, famName
		}
	}

	if !found {
		// Fall back to alias lookup so describe answers for any name a user
		// might reasonably type.
		if a, okAlias := cat.Alias(name); okAlias {
			describeAlias(cmd, a)
			return nil
		}
		return fmt.Errorf("no command or alias named %q (try 'loadout describe' with no argument)", name)
	}

	cmd.Printf("%s\n", resolvedName)
	if fn.Summary != "" {
		cmd.Printf("  %s\n", fn.Summary)
	}

	if fn.Usage != "" {
		cmd.Println("\nUsage")
		for _, line := range strings.Split(strings.TrimRight(fn.Usage, "\n"), "\n") {
			cmd.Printf("  %s\n", line)
		}
	}

	if len(fn.Config) > 0 {
		cmd.Println("\nConfigured by")
		for _, c := range fn.Config {
			cmd.Printf("  %s\n", c)
		}
	}

	specs := fn.ResolvedRequires(cfg.Paths.ProjectRoot, cfg.Paths.VenvRoot)
	if len(specs) > 0 {
		cmd.Println("\nRequires")
		chk := requires.New(presenceFunc(cfg))
		results, _ := chk.CheckAll(requires.MustParseAll(specs))
		for _, r := range results {
			mark := "x"
			if r.OK {
				mark = "y"
			}
			line := fmt.Sprintf("  [%s] %s", mark, r.Describe())
			if !haveConfig && r.Kind == requires.Config {
				line += "  (no config yet)"
			}
			cmd.Println(line)
		}
	}

	if fn.Notes != "" {
		cmd.Println("\nNotes")
		for _, line := range strings.Split(strings.TrimRight(fn.Notes, "\n"), "\n") {
			cmd.Printf("  %s\n", line)
		}
	}
	return nil
}

// familyFor resolves a name that comes from config rather than the catalog,
// returning the metadata entry that describes the family.
func familyFor(cat *catalog.Catalog, cfg *config.Config, name string) (catalog.Function, string, bool) {
	for _, f := range cat.Functions {
		if f.NamedBy != "clone_sources" {
			continue
		}
		for _, cs := range cfg.CloneSources {
			if cs.Alias == name {
				return f, fmt.Sprintf("%s  (clones from %s)", name, cs.Org), true
			}
		}
	}
	return catalog.Function{}, "", false
}

func describeAlias(cmd *cobra.Command, a catalog.Alias) {
	cmd.Printf("%s  (alias)\n", a.Name)
	if a.Comment != "" {
		cmd.Printf("  %s\n", a.Comment)
	}
	cmd.Println("\nExpands to")
	if len(a.Platforms) > 0 {
		oses := make([]string, 0, len(a.Platforms))
		for o := range a.Platforms {
			oses = append(oses, o)
		}
		sort.Strings(oses)
		for _, o := range oses {
			cmd.Printf("  %-8s %s\n", o, a.Platforms[o])
		}
	} else {
		cmd.Printf("  %s\n", a.Command)
	}
	cmd.Printf("\nCategory\n  %s\n", a.Category)
	if a.Pack != "" {
		cmd.Printf("\nPack\n  %s  (enable with: packs: [%s])\n", a.Pack, a.Pack)
	}
}
