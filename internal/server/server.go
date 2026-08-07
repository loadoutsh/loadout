// Package server hosts the local web UI.
//
// It listens on loopback only, for one user, for as long as the command runs.
// There is no daemon and no persistent surface. Because the API can rewrite
// the shell configuration the user's shell sources, every request is checked
// against the Host, the Origin, and a per-run token before it is served.
package server

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/wesscoby/loadout/internal/catalog"
	"github.com/wesscoby/loadout/internal/config"
	"github.com/wesscoby/loadout/internal/credentials"
	"github.com/wesscoby/loadout/internal/generate"
	"github.com/wesscoby/loadout/internal/requires"
)

//go:embed web/*
var webFS embed.FS

// Server is the local UI.
type Server struct {
	token string
	port  string

	cat    *catalog.Catalog
	binDir string

	// keyring is resolved lazily; a machine with no keychain should still get
	// a working UI for everything else.
	keyring credentials.Keyring
}

// New builds a Server.
func New(cat *catalog.Catalog, binDir string) (*Server, error) {
	tok, err := newToken()
	if err != nil {
		return nil, err
	}
	s := &Server{token: tok, cat: cat, binDir: binDir}
	if k, err := credentials.Open(); err == nil {
		s.keyring = k
	}
	return s, nil
}

// Serve runs until the context is cancelled. port 0 picks a free one.
func (s *Server) Serve(ctx context.Context, port int, onReady func(url string)) error {
	ln, err := listenLoopback(port)
	if err != nil {
		return err
	}
	s.port = portOf(ln)

	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return err
	}

	api := http.NewServeMux()
	api.HandleFunc("/api/state", s.handleState)
	api.HandleFunc("/api/toggle", s.handleToggle)
	api.HandleFunc("/api/diff", s.handleDiff)
	api.HandleFunc("/api/apply", s.handleApply)
	api.HandleFunc("/api/cred/use", s.handleCredUse)
	api.HandleFunc("/api/cred/set", s.handleCredSet)

	mux := http.NewServeMux()
	// The API needs the token; the static page does not, since a <script> tag
	// cannot present one. Both still require a loopback Host and our Origin.
	mux.Handle("/api/", s.guard(api))
	mux.Handle("/", s.baseGuard(http.FileServer(http.FS(sub))))

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	if onReady != nil {
		onReady(s.URL())
	}

	errCh := make(chan error, 1)
	go func() {
		err := srv.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

// --- payloads --------------------------------------------------------------

type aliasView struct {
	Name      string   `json:"name"`
	Command   string   `json:"command"`
	Category  string   `json:"category"`
	Pack      string   `json:"pack,omitempty"`
	Comment   string   `json:"comment,omitempty"`
	Requires  []string `json:"requires,omitempty"`
	Available bool     `json:"available"`
	Missing   []string `json:"missing,omitempty"`
	Enabled   bool     `json:"enabled"`
	Applies   bool     `json:"applies"`
}

type functionView struct {
	Name     string   `json:"name"`
	Summary  string   `json:"summary"`
	Usage    string   `json:"usage,omitempty"`
	Notes    string   `json:"notes,omitempty"`
	Config   []string `json:"config,omitempty"`
	Requires []string `json:"requires,omitempty"`
	Unmet    []string `json:"unmet,omitempty"`
	Ready    bool     `json:"ready"`
}

type packView struct {
	Name    string `json:"name"`
	Count   int    `json:"count"`
	Enabled bool   `json:"enabled"`
}

type credView struct {
	Name    string      `json:"name"`
	Active  string      `json:"active"`
	Entries []entryView `json:"entries"`
}

type entryView struct {
	Name     string `json:"name"`
	HasValue bool   `json:"has_value"`
}

type stateResponse struct {
	Platform    string         `json:"platform"`
	ConfigPath  string         `json:"config_path"`
	OutputPath  string         `json:"output_path"`
	BinDir      string         `json:"bin_dir"`
	Keyring     string         `json:"keyring"`
	ConfigError string         `json:"config_error,omitempty"`
	Aliases     []aliasView    `json:"aliases"`
	Functions   []functionView `json:"functions"`
	Packs       []packView     `json:"packs"`
	Credentials []credView     `json:"credentials"`
	Config      *config.Config `json:"config"`
	Stale       bool           `json:"stale"`
}

// --- handlers --------------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeJSON(w, http.StatusOK, stateResponse{ConfigError: err.Error()})
		return
	}

	chk := requires.New(presenceFunc(cfg))
	goos := runtime.GOOS

	enabledPacks := map[string]bool{}
	for _, p := range cfg.Packs {
		enabledPacks[p] = true
	}
	disabled := map[string]bool{}
	for _, n := range cfg.Aliases.Disabled {
		disabled[n] = true
	}

	resp := stateResponse{
		Platform: goos,
		BinDir:   s.binDir,
		Config:   cfg,
		Keyring:  "none available",
	}
	if s.keyring != nil {
		resp.Keyring = s.keyring.Name()
	}
	resp.ConfigPath, _ = config.Path()
	resp.OutputPath, _ = generate.OutputPath("zsh")
	resp.Stale = outputIsStale(resp.ConfigPath, resp.OutputPath)

	packCounts := map[string]int{}
	for _, a := range s.cat.Aliases {
		cmd, applies := a.Resolve(goos)
		reqs := s.cat.RequiredBinaries(a, goos)
		var missing []string
		for _, b := range reqs {
			if !chk.Check(requires.Requirement{Kind: requires.Binary, Value: b}).OK {
				missing = append(missing, b)
			}
		}
		if a.Pack != "" {
			packCounts[a.Pack]++
		}
		resp.Aliases = append(resp.Aliases, aliasView{
			Name:      a.Name,
			Command:   cmd,
			Category:  a.Category,
			Pack:      a.Pack,
			Comment:   a.Comment,
			Requires:  reqs,
			Available: len(missing) == 0,
			Missing:   missing,
			Enabled:   !disabled[a.Name] && (a.Pack == "" || enabledPacks[a.Pack]),
			Applies:   applies,
		})
	}

	for _, p := range s.cat.Packs() {
		resp.Packs = append(resp.Packs, packView{
			Name: p, Count: packCounts[p], Enabled: enabledPacks[p],
		})
	}

	for _, f := range s.cat.Functions {
		specs := f.ResolvedRequires(cfg.Paths.ProjectRoot, cfg.Paths.VenvRoot)
		results, allOK := chk.CheckAll(requires.MustParseAll(specs))
		var unmet []string
		for _, res := range results {
			if !res.OK {
				unmet = append(unmet, res.Describe())
			}
		}
		resp.Functions = append(resp.Functions, functionView{
			Name: f.Name, Summary: f.Summary, Usage: f.Usage, Notes: f.Notes,
			Config: f.Config, Requires: specs, Unmet: unmet, Ready: allOK,
		})
	}

	names := make([]string, 0, len(cfg.Credentials))
	for n := range cfg.Credentials {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		c := cfg.Credentials[n]
		cv := credView{Name: n, Active: c.Active}
		for _, e := range c.Entries {
			has := false
			if s.keyring != nil {
				// Only presence is reported. A value never leaves the
				// keychain for the UI, not even over loopback.
				if _, err := s.keyring.Get(credentials.Key(n, e)); err == nil {
					has = true
				}
			}
			cv.Entries = append(cv.Entries, entryView{Name: e, HasValue: has})
		}
		resp.Credentials = append(resp.Credentials, cv)
	}

	writeJSON(w, http.StatusOK, resp)
}

func outputIsStale(cfgPath, outPath string) bool {
	out, err := os.Stat(outPath)
	if err != nil {
		return true
	}
	c, err := os.Stat(cfgPath)
	if err != nil {
		return false
	}
	return c.ModTime().After(out.ModTime())
}

type toggleRequest struct {
	Kind    string `json:"kind"` // "alias" or "pack"
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

func (s *Server) handleToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("POST required"))
		return
	}
	var req toggleRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cfg, err := config.Load()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	switch req.Kind {
	case "alias":
		if _, ok := s.cat.Alias(req.Name); !ok {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("no alias %q in the catalog", req.Name))
			return
		}
		cfg.Aliases.Disabled = setMembership(cfg.Aliases.Disabled, req.Name, !req.Enabled)
	case "pack":
		known := false
		for _, p := range s.cat.Packs() {
			if p == req.Name {
				known = true
			}
		}
		if !known {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("no pack %q", req.Name))
			return
		}
		cfg.Packs = setMembership(cfg.Packs, req.Name, req.Enabled)
	default:
		writeErr(w, http.StatusBadRequest, fmt.Errorf("unknown kind %q", req.Kind))
		return
	}

	if err := cfg.Save(); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// setMembership adds or removes a value from a list, keeping it sorted and
// free of duplicates.
func setMembership(list []string, value string, present bool) []string {
	out := make([]string, 0, len(list)+1)
	for _, v := range list {
		if v != value {
			out = append(out, v)
		}
	}
	if present {
		out = append(out, value)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *Server) render() ([]byte, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	g, err := generate.New(s.cat, cfg, generate.Options{
		Shell: "zsh", GOOS: runtime.GOOS, BinDir: s.binDir,
	})
	if err != nil {
		return nil, err
	}
	return g.Render()
}

// handleDiff returns what is on disk and what would be written, so the user
// can see exactly what changes before anything touches their shell.
func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	next, err := s.render()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	outPath, err := generate.OutputPath("zsh")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	current, readErr := os.ReadFile(outPath)
	if readErr != nil {
		current = nil
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"current":   string(current),
		"next":      string(next),
		"changed":   string(current) != string(next),
		"path":      outPath,
		"no_output": readErr != nil,
	})
}

func (s *Server) handleApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("POST required"))
		return
	}
	rendered, err := s.render()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	outPath, err := generate.OutputPath("zsh")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := os.WriteFile(outPath, rendered, 0o644); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": outPath, "bytes": len(rendered)})
}

type credUseRequest struct {
	Name  string `json:"name"`
	Entry string `json:"entry"`
}

func (s *Server) handleCredUse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("POST required"))
		return
	}
	var req credUseRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cfg, err := config.Load()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	c, ok := cfg.Credentials[req.Name]
	if !ok {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("no credential %q", req.Name))
		return
	}
	found := false
	for _, e := range c.Entries {
		if e == req.Entry {
			found = true
		}
	}
	if !found {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("credential %q has no entry %q", req.Name, req.Entry))
		return
	}
	c.Active = req.Entry
	cfg.Credentials[req.Name] = c
	if err := cfg.Save(); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type credSetRequest struct {
	Name  string `json:"name"`
	Entry string `json:"entry"`
	Value string `json:"value"`
}

func (s *Server) handleCredSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("POST required"))
		return
	}
	if s.keyring == nil {
		writeErr(w, http.StatusBadRequest, errors.New("no keychain backend available on this machine"))
		return
	}
	var req credSetRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.Value == "" {
		writeErr(w, http.StatusBadRequest, errors.New("refusing to store an empty value"))
		return
	}
	cfg, err := config.Load()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	c, ok := cfg.Credentials[req.Name]
	if !ok {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("no credential %q", req.Name))
		return
	}
	found := false
	for _, e := range c.Entries {
		if e == req.Entry {
			found = true
		}
	}
	if !found {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("credential %q has no entry %q", req.Name, req.Entry))
		return
	}
	if err := s.keyring.Set(credentials.Key(req.Name, req.Entry), req.Value); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// presenceFunc mirrors the doctor command's view of which config sections
// count as populated.
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
