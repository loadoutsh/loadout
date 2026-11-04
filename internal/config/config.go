// Package config defines the user's loadout configuration: the personal
// values that the shipped catalog is rendered against.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// CurrentVersion is the schema version this build writes and understands.
const CurrentVersion = 1

// Config is the whole of ~/.config/loadout/config.yaml.
type Config struct {
	Version int `yaml:"version"`

	Paths        Paths                 `yaml:"paths"`
	Git          Git                   `yaml:"git"`
	SSH          SSH                   `yaml:"ssh"`
	Identities   []Identity            `yaml:"identities,omitempty"`
	Credentials  map[string]Credential `yaml:"credentials,omitempty"`
	Hosts        []Host                `yaml:"hosts,omitempty"`
	Workspaces   []Workspace           `yaml:"workspaces,omitempty"`
	CloneSources []CloneSource         `yaml:"clone_sources,omitempty"`
	Editors      []Editor              `yaml:"editors,omitempty"`
	NodeVersions []string              `yaml:"node_versions,omitempty"`
	NpmGlobals   []string              `yaml:"npm_globals,omitempty"`
	Binaries     []Binary              `yaml:"binaries,omitempty"`
	Scripts      []Script              `yaml:"scripts,omitempty"`
	Packs        []string              `yaml:"packs,omitempty"`
	Aliases      Aliases               `yaml:"aliases"`
}

// Paths are the roots the navigation and virtualenv functions operate on.
type Paths struct {
	ProjectRoot string `yaml:"project_root"`
	VenvRoot    string `yaml:"venv_root"`
}

// Git holds the branch names baked into the git aliases.
type Git struct {
	MainBranch string `yaml:"main_branch"`
	DevBranch  string `yaml:"dev_branch"`
}

// SSH holds the defaults the msh helper falls back to.
type SSH struct {
	DefaultKey  string `yaml:"default_key,omitempty"`
	DefaultUser string `yaml:"default_user,omitempty"`
	DefaultHost string `yaml:"default_host,omitempty"`
	DefaultPort int    `yaml:"default_port,omitempty"`

	// SubnetPrefix, when set, lets a bare number be given as the last octet.
	// "10.0.0." plus "5" means 10.0.0.5. Empty disables the shorthand.
	SubnetPrefix string `yaml:"subnet_prefix,omitempty"`
}

// Identity is one git author plus the SSH key and credentials that go with it.
type Identity struct {
	ID      string `yaml:"id"`
	GitUser string `yaml:"git_user"`
	Email   string `yaml:"email"`
	// SSHKey is optional. When it is empty, `s` leaves the SSH agent alone,
	// for keys that ~/.ssh/config already picks, such as hardware- or
	// TPM-backed ones held by an agent `ssh-add` would never reach.
	SSHKey    string            `yaml:"ssh_key,omitempty"`
	Default   bool              `yaml:"default,omitempty"`
	Activates map[string]string `yaml:"activates,omitempty"`
}

// Credential names the entries stored for one secret and which is live.
// Values are never held here; they live in the OS keychain.
type Credential struct {
	Active  string   `yaml:"active"`
	Entries []string `yaml:"entries"`

	// Env overrides the environment variable this credential exports as.
	// Empty derives it from the key: npm_token becomes NPM_TOKEN. Set it when
	// the consuming tool expects a name the key does not produce, which is
	// common enough that deriving it was a mistake to make unavoidable.
	Env string `yaml:"env,omitempty"`
}

// EnvName is the variable this credential exports as.
func (c Credential) EnvName(key string) string {
	if c.Env != "" {
		return c.Env
	}
	return DeriveEnvName(key)
}

// DeriveEnvName is the default mapping from a credential key to a variable
// name: upper-cased, with separators normalised to underscores.
func DeriveEnvName(key string) string {
	return strings.ToUpper(strings.NewReplacer("-", "_", ".", "_", "/", "_").Replace(key))
}

// Host is a remote machine reachable over SSH, used by msh and mpush.
type Host struct {
	Alias      string `yaml:"alias"`
	Hostname   string `yaml:"hostname"`
	User       string `yaml:"user"`
	Port       int    `yaml:"port,omitempty"`
	Key        string `yaml:"key,omitempty"`
	Password   string `yaml:"password,omitempty"` // "cred:<name>", never a literal
	UploadPath string `yaml:"upload_path,omitempty"`
	Shortcut   bool   `yaml:"shortcut,omitempty"` // also emit a bare `ssh <alias>` alias
}

// Workspace is an editor workspace, optionally paired with a tmux session.
type Workspace struct {
	Name        string `yaml:"name"`
	Path        string `yaml:"path"`
	TmuxSession string `yaml:"tmux_session,omitempty"`
}

// CloneSource maps a short command onto a git host organisation.
type CloneSource struct {
	Alias string `yaml:"alias"`
	Org   string `yaml:"org"`
}

// Editor is a launchable editor binary. The first entry is the default.
type Editor struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
}

// Binary is a third-party executable fetched per platform into the managed
// bin directory. Never committed to a repository.
type Binary struct {
	Name     string       `yaml:"name"`
	Version  string       `yaml:"version"`
	Source   BinarySource `yaml:"source"`
	Checksum Checksum     `yaml:"checksum,omitempty"`
}

// Checksum pins the expected hash of a download.
//
// A hash only describes one platform's artefact, so a bare string would be
// wrong everywhere except the machine it was taken on. Both forms are
// accepted: a single string when only one platform is used, or a map keyed by
// "<os>-<arch>" when the binary is declared for several.
//
//	checksum: sha256:abc...
//
//	checksum:
//	  linux-amd64: sha256:abc...
//	  darwin-arm64: sha256:def...
type Checksum struct {
	// Any applies to every platform. Empty when per-platform hashes are used.
	Any string
	// ByPlatform is keyed by "<os>-<arch>".
	ByPlatform map[string]string
}

// UnmarshalYAML accepts either a scalar or a mapping.
func (c *Checksum) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		return value.Decode(&c.Any)
	case yaml.MappingNode:
		return value.Decode(&c.ByPlatform)
	default:
		return fmt.Errorf("checksum must be a string or a map keyed by platform")
	}
}

// MarshalYAML writes back whichever form was given.
func (c Checksum) MarshalYAML() (any, error) {
	if len(c.ByPlatform) > 0 {
		return c.ByPlatform, nil
	}
	if c.Any != "" {
		return c.Any, nil
	}
	return nil, nil
}

// For returns the expected hash for a platform key, and whether one is pinned.
func (c Checksum) For(platformKey string) (string, bool) {
	if v, ok := c.ByPlatform[platformKey]; ok && v != "" {
		return v, true
	}
	if c.Any != "" {
		return c.Any, true
	}
	return "", false
}

// IsZero reports whether nothing is pinned at all.
func (c Checksum) IsZero() bool { return c.Any == "" && len(c.ByPlatform) == 0 }

// BinarySource is either a GitHub release or a set of direct URLs, keyed by
// "<os>-<arch>".
type BinarySource struct {
	GitHub string            `yaml:"github,omitempty"`
	Asset  map[string]string `yaml:"asset,omitempty"`
	URL    map[string]string `yaml:"url,omitempty"`
}

// Script is an executable text file placed on PATH, either shipped with
// loadout or symlinked from somewhere the user keeps it.
type Script struct {
	Name   string `yaml:"name"`
	Source string `yaml:"source"` // "builtin" or a path
}

// Aliases carries the user's opt-outs and their own additions.
type Aliases struct {
	Disabled []string      `yaml:"disabled,omitempty"`
	Custom   []CustomAlias `yaml:"custom,omitempty"`
}

// CustomAlias is a user-defined alias, optionally limited to some platforms.
type CustomAlias struct {
	Name     string   `yaml:"name"`
	Command  string   `yaml:"command"`
	Platform []string `yaml:"platform,omitempty"`
}

// Default returns a config with the values a fresh install starts from.
func Default() *Config {
	return &Config{
		Version: CurrentVersion,
		Paths: Paths{
			ProjectRoot: "~/Projects",
			VenvRoot:    "~/.venv",
		},
		Git: Git{
			MainBranch: "main",
			DevBranch:  "development",
		},
		SSH: SSH{
			DefaultUser: "root",
			DefaultPort: 22,
		},
		NodeVersions: []string{"20", "22", "24"},
	}
}

// Dir is ~/.config/loadout, honouring XDG_CONFIG_HOME when set.
func Dir() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "loadout"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "loadout"), nil
}

// Path is the location of config.yaml.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// LocalPath is the never-synced overlay applied on top of config.yaml.
func LocalPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "local.yaml"), nil
}

// Load reads config.yaml, applies local.yaml over it when present, fills in
// defaults and validates the result.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	cfg, err := loadFile(path)
	if err != nil {
		return nil, err
	}

	localPath, err := LocalPath()
	if err != nil {
		return nil, err
	}
	// The overlay is optional; its absence is the normal case.
	if _, statErr := os.Stat(localPath); statErr == nil {
		local, err := loadFile(localPath)
		if err != nil {
			return nil, fmt.Errorf("local overlay: %w", err)
		}
		cfg.merge(local)
	}

	cfg.applyDefaults()
	return cfg, cfg.Validate()
}

func loadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no config at %s: run 'loadout init'", path)
		}
		return nil, err
	}
	var cfg Config
	// KnownFields catches typo'd keys instead of silently ignoring them, which
	// otherwise shows up as a setting that mysteriously has no effect.
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &cfg, nil
}

// merge applies a local overlay. Scalars override when set; lists replace
// wholesale rather than concatenating, so an overlay can shorten a list.
func (c *Config) merge(o *Config) {
	if o.Paths.ProjectRoot != "" {
		c.Paths.ProjectRoot = o.Paths.ProjectRoot
	}
	if o.Paths.VenvRoot != "" {
		c.Paths.VenvRoot = o.Paths.VenvRoot
	}
	if o.Git.MainBranch != "" {
		c.Git.MainBranch = o.Git.MainBranch
	}
	if o.Git.DevBranch != "" {
		c.Git.DevBranch = o.Git.DevBranch
	}
	if o.Identities != nil {
		c.Identities = o.Identities
	}
	if o.Hosts != nil {
		c.Hosts = o.Hosts
	}
	if o.Workspaces != nil {
		c.Workspaces = o.Workspaces
	}
	if o.CloneSources != nil {
		c.CloneSources = o.CloneSources
	}
	if o.Editors != nil {
		c.Editors = o.Editors
	}
	if o.NodeVersions != nil {
		c.NodeVersions = o.NodeVersions
	}
	if o.NpmGlobals != nil {
		c.NpmGlobals = o.NpmGlobals
	}
	if o.Binaries != nil {
		c.Binaries = o.Binaries
	}
	if o.Scripts != nil {
		c.Scripts = o.Scripts
	}
	if o.Packs != nil {
		c.Packs = o.Packs
	}
	if o.Credentials != nil {
		if c.Credentials == nil {
			c.Credentials = map[string]Credential{}
		}
		for k, v := range o.Credentials {
			c.Credentials[k] = v
		}
	}
	if o.Aliases.Disabled != nil {
		c.Aliases.Disabled = o.Aliases.Disabled
	}
	if o.Aliases.Custom != nil {
		c.Aliases.Custom = o.Aliases.Custom
	}
}

func (c *Config) applyDefaults() {
	d := Default()
	if c.Version == 0 {
		c.Version = d.Version
	}
	if c.Paths.ProjectRoot == "" {
		c.Paths.ProjectRoot = d.Paths.ProjectRoot
	}
	if c.Paths.VenvRoot == "" {
		c.Paths.VenvRoot = d.Paths.VenvRoot
	}
	if c.Git.MainBranch == "" {
		c.Git.MainBranch = d.Git.MainBranch
	}
	if c.Git.DevBranch == "" {
		c.Git.DevBranch = d.Git.DevBranch
	}
	if c.SSH.DefaultUser == "" {
		c.SSH.DefaultUser = d.SSH.DefaultUser
	}
	if c.SSH.DefaultPort == 0 {
		c.SSH.DefaultPort = d.SSH.DefaultPort
	}
	for i := range c.Hosts {
		if c.Hosts[i].Port == 0 {
			c.Hosts[i].Port = 22
		}
		if c.Hosts[i].UploadPath == "" {
			c.Hosts[i].UploadPath = "~"
		}
	}
}

// Validate reports configuration that would generate broken shell.
func (c *Config) Validate() error {
	if c.Version > CurrentVersion {
		return fmt.Errorf("config version %d is newer than this build understands (%d)", c.Version, CurrentVersion)
	}

	defaults := 0
	seenIdentity := map[string]bool{}
	for _, id := range c.Identities {
		if id.ID == "" {
			return fmt.Errorf("identity with no id")
		}
		if seenIdentity[id.ID] {
			return fmt.Errorf("duplicate identity %q", id.ID)
		}
		seenIdentity[id.ID] = true
		if id.Default {
			defaults++
		}
		for cred, entry := range id.Activates {
			c, ok := c.Credentials[cred]
			if !ok {
				return fmt.Errorf("identity %q activates unknown credential %q", id.ID, cred)
			}
			if !contains(c.Entries, entry) {
				return fmt.Errorf("identity %q activates %q entry %q, which is not declared", id.ID, cred, entry)
			}
		}
	}
	if defaults > 1 {
		return fmt.Errorf("%d identities marked default, expected at most 1", defaults)
	}

	envNames := map[string]string{}
	for name, cred := range c.Credentials {
		if cred.Active != "" && !contains(cred.Entries, cred.Active) {
			return fmt.Errorf("credential %q is active on %q, which is not in its entries", name, cred.Active)
		}
		// Two credentials exporting the same variable means one silently
		// overwrites the other, with the winner decided by map iteration.
		env := cred.EnvName(name)
		if other, clash := envNames[env]; clash {
			return fmt.Errorf("credentials %q and %q both export %s; set a distinct \"env\" on one of them",
				other, name, env)
		}
		envNames[env] = name
	}

	seenHost := map[string]bool{}
	for _, h := range c.Hosts {
		if h.Alias == "" {
			return fmt.Errorf("host with no alias")
		}
		if seenHost[h.Alias] {
			return fmt.Errorf("duplicate host %q", h.Alias)
		}
		seenHost[h.Alias] = true
		if h.Hostname == "" {
			return fmt.Errorf("host %q has no hostname", h.Alias)
		}
		// A literal password in the config file is exactly what the keychain
		// exists to prevent, so reject it rather than quietly writing it out.
		if h.Password != "" && !strings.HasPrefix(h.Password, "cred:") {
			return fmt.Errorf("host %q: password must reference a credential as \"cred:<name>\", not a literal", h.Alias)
		}
	}

	seenWorkspace := map[string]bool{}
	for _, w := range c.Workspaces {
		if w.Name == "" {
			return fmt.Errorf("workspace with no name")
		}
		if seenWorkspace[w.Name] {
			return fmt.Errorf("duplicate workspace %q", w.Name)
		}
		seenWorkspace[w.Name] = true
		if w.Path == "" {
			return fmt.Errorf("workspace %q has no path", w.Name)
		}
	}

	return nil
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// Save writes the config to disk, creating the directory if needed.
func (c *Config) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
