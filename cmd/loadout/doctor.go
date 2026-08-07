package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wesscoby/loadout/internal/catalog"
	"github.com/wesscoby/loadout/internal/config"
	"github.com/wesscoby/loadout/internal/generate"
	"github.com/wesscoby/loadout/internal/requires"
)

// severity of one finding.
type severity int

const (
	ok severity = iota
	warn
	bad
)

func (s severity) label() string {
	switch s {
	case ok:
		return "ok  "
	case warn:
		return "warn"
	default:
		return "FAIL"
	}
}

type finding struct {
	sev    severity
	title  string
	detail string
}

// report accumulates findings under section headings.
type report struct {
	sections []section
}

type section struct {
	name     string
	findings []finding
}

func (r *report) add(sectionName string, f finding) {
	for i := range r.sections {
		if r.sections[i].name == sectionName {
			r.sections[i].findings = append(r.sections[i].findings, f)
			return
		}
	}
	r.sections = append(r.sections, section{name: sectionName, findings: []finding{f}})
}

func (r *report) worst() severity {
	w := ok
	for _, s := range r.sections {
		for _, f := range s.findings {
			if f.sev > w {
				w = f.sev
			}
		}
	}
	return w
}

// write prints the report. Passing checks are summarised rather than listed,
// so the output is about what needs attention.
func (r *report) write(w io.Writer, verbose bool) {
	for _, s := range r.sections {
		var shown []finding
		passed := 0
		for _, f := range s.findings {
			if f.sev == ok && !verbose {
				passed++
				continue
			}
			shown = append(shown, f)
		}
		if len(shown) == 0 && passed == 0 {
			continue
		}

		fmt.Fprintf(w, "\n%s\n", s.name)
		for _, f := range shown {
			fmt.Fprintf(w, "  [%s] %s\n", f.sev.label(), f.title)
			for _, line := range strings.Split(strings.TrimRight(f.detail, "\n"), "\n") {
				if line != "" {
					fmt.Fprintf(w, "         %s\n", line)
				}
			}
		}
		if passed > 0 {
			fmt.Fprintf(w, "  [ok  ] %d further check(s) passed\n", passed)
		}
	}
}

func newDoctorCmd() *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check that everything your config asks for actually works here",
		Long: `Report anything that would stop your configuration working on this machine:
missing tools, paths that do not exist, requirements a command needs before it
can run, and generated output that has fallen behind your config.

Warnings exit 0. Failures exit 1.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			rep := &report{}

			cfg, cfgErr := config.Load()
			checkConfig(rep, cfg, cfgErr)
			if cfgErr != nil {
				rep.write(out, verbose)
				fmt.Fprintln(out, "\nfix the config before the rest can be checked")
				return errSilent
			}

			cat, err := catalog.Load()
			if err != nil {
				return err
			}

			chk := requires.New(presenceFunc(cfg))
			checkGenerated(rep, cfg)
			checkShellIntegration(rep)
			checkPaths(rep, cfg, chk)
			checkFunctions(rep, cat, cfg, chk)
			checkAliases(rep, cat, cfg, chk)
			checkBinDir(rep)

			rep.write(out, verbose)

			switch rep.worst() {
			case bad:
				fmt.Fprintln(out, "\nsome checks failed")
				return errSilent
			case warn:
				fmt.Fprintln(out, "\nusable, with warnings above")
			default:
				fmt.Fprintln(out, "\neverything checks out")
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "list passing checks too")
	return cmd
}

// errSilent signals a non-zero exit without cobra printing another message,
// since doctor has already reported everything in its own format.
var errSilent = fmt.Errorf("")

// presenceFunc reports whether a named config section holds anything.
func presenceFunc(cfg *config.Config) requires.ConfigPresence {
	return func(s string) (bool, bool) {
		switch s {
		case "identities":
			return len(cfg.Identities) > 0, true
		case "hosts":
			return len(cfg.Hosts) > 0, true
		case "workspaces":
			return len(cfg.Workspaces) > 0, true
		case "clone_sources":
			return len(cfg.CloneSources) > 0, true
		case "editors":
			return len(cfg.Editors) > 0, true
		case "npm_globals":
			return len(cfg.NpmGlobals) > 0, true
		case "binaries":
			return len(cfg.Binaries) > 0, true
		default:
			return false, false
		}
	}
}

func checkConfig(rep *report, cfg *config.Config, err error) {
	if err != nil {
		rep.add("Configuration", finding{bad, "config could not be loaded", err.Error()})
		return
	}
	path, _ := config.Path()
	rep.add("Configuration", finding{ok, "config is valid", path})
	if cfg.Version < config.CurrentVersion {
		rep.add("Configuration", finding{warn,
			fmt.Sprintf("config is version %d, current is %d", cfg.Version, config.CurrentVersion),
			"run 'loadout apply' to migrate it"})
	}
}

// checkGenerated catches the most common confusion: config edited, apply not
// run, and the shell still sourcing yesterday's output.
func checkGenerated(rep *report, cfg *config.Config) {
	genPath, err := generate.OutputPath("zsh")
	if err != nil {
		rep.add("Generated output", finding{bad, "cannot determine output path", err.Error()})
		return
	}
	genInfo, err := os.Stat(genPath)
	if err != nil {
		rep.add("Generated output", finding{bad, "no generated file", "run 'loadout apply'"})
		return
	}
	cfgPath, _ := config.Path()
	cfgInfo, err := os.Stat(cfgPath)
	if err == nil && cfgInfo.ModTime().After(genInfo.ModTime()) {
		rep.add("Generated output", finding{warn, "generated file is older than your config",
			"run 'loadout apply' to pick up the changes"})
		return
	}
	rep.add("Generated output", finding{ok, "generated file is current", genPath})
}

// checkShellIntegration looks for the source line in the user's shell rc.
func checkShellIntegration(rep *report) {
	genPath, err := generate.OutputPath("zsh")
	if err != nil {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	rc := filepath.Join(home, ".zshrc")
	data, err := os.ReadFile(rc)
	if err != nil {
		rep.add("Shell integration", finding{warn, "could not read ~/.zshrc", err.Error()})
		return
	}
	// Match on the file name rather than the whole path, so a config dir moved
	// via XDG_CONFIG_HOME does not read as missing integration.
	if strings.Contains(string(data), filepath.Base(genPath)) {
		rep.add("Shell integration", finding{ok, "~/.zshrc sources the generated file", ""})
		return
	}
	rep.add("Shell integration", finding{warn, "~/.zshrc does not source the generated file",
		fmt.Sprintf("add:  source %s", genPath)})
}

func checkPaths(rep *report, cfg *config.Config, chk *requires.Checker) {
	type target struct {
		label string
		path  string
		sev   severity
	}
	targets := []target{
		{"projects root", cfg.Paths.ProjectRoot, warn},
		{"venv root", cfg.Paths.VenvRoot, warn},
	}
	for _, w := range cfg.Workspaces {
		targets = append(targets, target{"workspace " + w.Name, w.Path, warn})
	}
	for _, id := range cfg.Identities {
		if id.SSHKey != "" {
			targets = append(targets, target{"ssh key for identity " + id.ID, id.SSHKey, warn})
		}
	}
	for _, h := range cfg.Hosts {
		if h.Key != "" {
			targets = append(targets, target{"ssh key for host " + h.Alias, h.Key, warn})
		}
	}

	for _, t := range targets {
		if t.path == "" {
			continue
		}
		res := chk.Check(requires.Requirement{Kind: requires.Path, Value: t.path})
		if res.OK {
			rep.add("Paths", finding{ok, t.label, t.path})
			continue
		}
		rep.add("Paths", finding{t.sev, t.label + " does not exist", requires.Expand(t.path)})
	}
}

func checkFunctions(rep *report, cat *catalog.Catalog, cfg *config.Config, chk *requires.Checker) {
	fns := make([]catalog.Function, len(cat.Functions))
	copy(fns, cat.Functions)
	sort.Slice(fns, func(i, j int) bool { return fns[i].Name < fns[j].Name })

	for _, fn := range fns {
		specs := fn.ResolvedRequires(cfg.Paths.ProjectRoot, cfg.Paths.VenvRoot)
		if len(specs) == 0 {
			continue
		}
		results, allOK := chk.CheckAll(requires.MustParseAll(specs))
		if allOK {
			rep.add("Commands", finding{ok, fn.Name, fn.Summary})
			continue
		}
		var unmet []string
		for _, r := range results {
			if !r.OK {
				unmet = append(unmet, r.Describe())
			}
		}
		rep.add("Commands", finding{warn,
			fmt.Sprintf("%s is not ready", fn.Name),
			strings.Join(unmet, "\n") + "\nsee 'loadout describe " + fn.Name + "'"})
	}
}

// checkAliases summarises rather than listing, since a hundred-odd aliases
// would bury everything else.
func checkAliases(rep *report, cat *catalog.Catalog, cfg *config.Config, chk *requires.Checker) {
	packs := map[string]bool{}
	for _, p := range cfg.Packs {
		packs[p] = true
	}
	disabled := map[string]bool{}
	for _, n := range cfg.Aliases.Disabled {
		disabled[n] = true
	}

	missing := map[string][]string{} // binary -> aliases needing it
	active := 0
	for _, a := range cat.Aliases {
		if disabled[a.Name] {
			continue
		}
		if a.Pack != "" && !packs[a.Pack] {
			continue
		}
		if _, applies := a.Resolve(runtime.GOOS); !applies {
			continue
		}
		active++
		for _, bin := range cat.RequiredBinaries(a, runtime.GOOS) {
			res := chk.Check(requires.Requirement{Kind: requires.Binary, Value: bin})
			if !res.OK {
				missing[bin] = append(missing[bin], a.Name)
			}
		}
	}

	if len(missing) == 0 {
		rep.add("Aliases", finding{ok, fmt.Sprintf("all %d active aliases have their tools", active), ""})
		return
	}

	bins := make([]string, 0, len(missing))
	for b := range missing {
		bins = append(bins, b)
	}
	sort.Strings(bins)

	var lines []string
	affected := 0
	for _, b := range bins {
		sort.Strings(missing[b])
		affected += len(missing[b])
		lines = append(lines, fmt.Sprintf("%-16s %s", b, strings.Join(missing[b], " ")))
	}
	rep.add("Aliases", finding{warn,
		fmt.Sprintf("%d of %d active aliases need a tool that is not installed", affected, active),
		strings.Join(lines, "\n")})
}

func checkBinDir(rep *report) {
	dir, err := managedBinDir()
	if err != nil {
		return
	}
	if _, err := os.Stat(dir); err != nil {
		// Nothing installed yet is normal, not a problem.
		return
	}
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if p == dir {
			rep.add("Managed binaries", finding{ok, "bin directory is on PATH", dir})
			return
		}
	}
	rep.add("Managed binaries", finding{warn, "bin directory exists but is not on PATH",
		dir + "\nrun 'loadout apply' and reload your shell"})
}
