# Design

Turning this personal dotfiles repo into an installable CLI application for Linux and macOS, with a local web UI for managing it.

Status: **All seven phases complete.** Open questions are collected at the end.

## What we are building

A curated, opinionated shell environment that other people can install, plus the tooling to personalise it. The alias and function set is the product; the values currently hardcoded into that set (identities, project paths, remote hosts) become user configuration.

## Decisions

| Decision          | Choice                                | Why                                                                                                                   |
| ----------------- | ------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| Product scope     | Curated defaults, user-personalisable | The taste is the value. Personal values are data.                                                                     |
| Language          | Go                                    | Single static binary, cross-compiles to both platforms, `go:embed` ships the web UI inside the binary.                |
| Web UI            | On-demand localhost server            | No daemon, no background surface. `loadout ui` starts it, closing it stops it.                                        |
| Shell integration | Generated file, sourced               | Zero process spawn per shell start, and `p`/`activate` stay real shell functions.                                     |
| Shells            | zsh and bash, pluggable backends      | The interface existed from day one, so adding bash was a new file rather than a rewrite.                              |
| Secrets           | OS keychain                           | Keychain on macOS, libsecret on Linux. Secrets never land in a file.                                                  |
| Name              | `loadout`                             | A deliberately configured kit you swap per context. No collision on PATH, in apt, or in brew. Searchable and ownable. |
| Repo              | New repo, fresh history               | Leaves 61MB of committed binaries behind in the archive rather than carrying them forward.                            |
| Sync              | Git-backed config repo                | Config is text. Credentials stay per-machine because a keychain is deliberately non-exportable.                       |
| Migration         | Schema migration only                 | No importer for the old shell setup: it has exactly one user. Hand-write the config once, use it as the parity test.  |

## Architecture

The core insight: **an alias cannot be a binary.** `gpush` and `..` are shell state, not programs. So the tool must emit shell no matter what it is written in. Once it is a generator, keeping the functions as generated shell too is the consistent choice, and it keeps the binary out of the hot path.

```
~/.config/loadout/config.yaml   ──►   loadout apply   ──►   ~/.config/loadout/init.zsh
        ▲                                                              │
        │                                                              ▼
   loadout CLI  ◄──►  web UI (127.0.0.1)                     sourced by ~/.zshrc
```

The CLI and the web UI are two front-ends over one config file. Neither runs when you open a shell.

Only two of the fourteen current functions mutate the calling shell: `p` (cd) and `activate` (source). Because they stay generated shell functions rather than becoming subcommands, no eval bridge is needed. Everything else could be a binary subcommand if a reason ever appears; `s` included, since `ssh-add` reaches the agent over `$SSH_AUTH_SOCK` and works from a child process.

### Product versus user data

A clean split, and the reason emails can sit in plaintext config:

- **This repo** ships the product: Go source, the curated alias and function templates, the web UI.
- **`~/.config/loadout/`** holds the user's data: `config.yaml`, generated `init.zsh`, state.
- **OS keychain** holds credentials only.

Nothing personal is committed to this repo. The test fixture and every example use illustrative values; the only exceptions are in this document and the triage record, where naming the actual aliases that were migrated is the point.

## Repo layout

Built so far marked with a tick.

```
cmd/loadout/           entry point, cobra commands          ✓
internal/
  config/              schema, load, merge, validate        ✓
  catalog/             curated set, embedded                ✓
    data/aliases.yaml    136 shipped aliases                ✓
    data/functions/      14 function templates              ✓
  generate/            rendering                            ✓
    backend/           Backend interface; zsh               ✓
  binaries/            fetch, verify, extract, install, state       ✓
  credentials/         keychain: security, secret-tool, pass       ✓
  requires/            capability checks for doctor and describe    ✓
  server/              localhost web UI + its security            ✓
    web/               embedded page, no build step                ✓
  sync/                git-backed config repo + secret scan         ✓
testdata/fixture/      config exercising every schema section  ✓
testdata/golden/       pinned output, 2 shells x 2 platforms   ✓
docs/
```

## Config schema

Sketch, to make the decisions concrete. Format is YAML.

```yaml
version: 1

paths:
  project_root: ~/Projects
  venv_root: ~/.venv

identities:
  - id: wc
    git_user: Wess
    email: wess@example.com
    ssh_key: ~/.ssh/wc_ed
    # Switching identity can also switch which credentials are active
    activates:
      npm_token: personal
      gh_token: personal

credentials:
  npm_token:
    active: personal
    entries: [personal, work, org] # values live in the keychain, never here
  gh_token:
    active: personal
    entries: [personal, work]

hosts:
  - alias: api
    hostname: 10.0.0.5
    user: root
    port: 22
    auth: { key: ~/.ssh/deploy } # or { password: cred:host_password }
    upload_path: ~

workspaces:
  - name: a11
    path: ~/Projects/acme/api/api.code-workspace
    tmux_session: a11 # generates the `a11` alias: pw a11 && ta a11

clone_sources:
  - alias: wt
    org: example-org

npm_globals: [pm2, serve, prettier, "@nestjs/cli"]

# Third-party binaries, fetched per platform. Never committed.
binaries:
  - name: duckdb
    version: v1.1.3
    source:
      github: duckdb/duckdb
      asset:
        linux-amd64: duckdb_cli-linux-amd64.zip
        darwin-arm64: duckdb_cli-osx-universal.zip
    checksum: sha256:...
  - name: ngrok
    version: 3.5.0
    source:
      url:
        linux-amd64: https://bin.equinox.io/c/.../ngrok-v3-stable-linux-amd64.tgz
        darwin-arm64: https://bin.equinox.io/c/.../ngrok-v3-stable-darwin-arm64.zip

# Executable scripts. Text, versioned, either shipped or the user's own.
scripts:
  - name: csf
    source: builtin
  - name: tmux-session
    source: builtin
  - name: deploy
    source: ~/dev/scripts/deploy.sh # symlinked, stays where it lives

# Opt-in bundles for one tool or ecosystem. Off unless listed.
packs: [flutter, cloudflared]

aliases:
  disabled: [ls, dl] # opt out of curated entries
  custom:
    - name: deploy
      command: ./scripts/deploy.sh
      platform: [linux]
```

Aliases sort into four buckets: shipped defaults, generated-from-config, opt-in packs, and dropped. See [ALIAS-TRIAGE.md](ALIAS-TRIAGE.md) for the pass over all 188.

Credentials store only names and which one is active. Values are keychain-backed, addressed as `loadout/<credential>/<entry>`.

### How secrets reach the shell

They are deliberately **not** written into the generated file, because that file is plain text on disk. Something therefore has to run to fetch them:

```sh
eval "$(loadout env)"
```

That is a real cost — one subprocess, and possibly a keychain unlock prompt — which is exactly why it is not added to shell startup automatically. Put it in your shell config only if you want tokens present in every shell. The trade is explicit rather than hidden.

The output is `eval`-ed, so every value is single-quoted with embedded quotes escaped; a token containing `$(...)` or backticks cannot execute anything.

Backends are tried in order and the first available wins: `security` on macOS, `secret-tool` (libsecret) on Linux, then `pass`. When none is present the error names what to install rather than just failing, which matters on a server where none of them exist.

## Binaries and scripts

`~/.bash/dotfiles/bin` is on `PATH` today via a hand-added line in `.zshrc`. That becomes a managed feature: the tool owns a bin directory, keeps it on `PATH`, and installs things into it.

The current `bin/` conflates two kinds of thing that need entirely different handling.

**Scripts** (`csf`, `sshp`, `tmux-session`, `fmt`) are text you authored. They are version-controlled, ship as builtins or point at a user file, and are platform-independent. Installing one is a symlink.

**Third-party binaries** (`duckdb`, `ngrok`) are downloaded release artifacts. They are platform-specific, need version pinning, and must never be committed. Installing one is a fetch, verify, extract, `chmod +x`.

That distinction is what makes the 61MB problem go away. A committed `linux-x86_64` duckdb is dead weight on a Mac; a declared one resolves to the right asset per platform, and the repo carries a few lines of YAML instead of 40MB.

```sh
loadout bin list              # installed, versions, updates available
loadout bin add duckdb/duckdb # resolve latest release, pin it
loadout bin update [name]
loadout bin remove <name>
```

Install target is `~/.local/share/loadout/bin`, added to `PATH` by the generated `init.zsh`. **Appended, not prepended**, matching today's behaviour so a managed binary cannot silently shadow a system one. Configurable for people who want the pinned version to win.

Two things this earns beyond tidiness. Version drift becomes visible: the committed ngrok is from March 2023 and duckdb from November 2023, and neither announces itself as stale. And it gives macOS support somewhere to live, since the same declaration resolves to a different asset per platform.

Anything installed here lands on `PATH` and will be executed, so this is the one feature that downloads and runs code. What that means in practice:

- HTTPS only, **including every redirect hop**. Checking just the configured URL is not enough, because release hosts redirect to a CDN.
- A declared checksum is verified before anything is installed, and `bin list` says plainly whether each binary was verified or merely downloaded.
- Archive members with absolute or `..` paths are refused, and extraction is size-capped.
- Writes go to a temporary file and are renamed into place, so an interrupted install cannot leave a half-written executable on PATH.
- Provenance is recorded: `bin list` shows the exact URL each binary came from.

## Sync

Config is text and syncs trivially. Credentials do not sync at all: a keychain is per-machine and deliberately non-exportable. So this splits into two problems with different answers.

**Config** syncs through a git-backed private repo the tool drives. It shells out to `git`, so existing SSH keys and credential helpers work unchanged.

```sh
loadout sync init git@github.com:user/loadout-config.git
loadout sync status    # exactly which files would leave the machine
loadout sync push
loadout sync pull
loadout scan           # check for credentials without touching git
```

Only `config.yaml` is tracked. `init.*` is generated and would conflict on every machine; `local.yaml` is the machine-specific overlay and exists precisely to stay put.

A scanner runs **before the commit**, not before the push, so a credential pasted into the config never enters the history at all — once committed, removing it needs a rewrite. It matches high-signal shapes (`ghp_`, `npm_`, `AKIA`, JWTs, PEM blocks, literal `password:` fields) rather than guessing at entropy, which would fire constantly on hostnames and paths. `password: cred:<name>` is the correct form and is explicitly allowed.

**Credentials** get one of three treatments, and the choice is the user's:

| Approach                   | Trade                                                                            |
| -------------------------- | -------------------------------------------------------------------------------- |
| Re-enter per machine       | Default. Safe, no dependency, mildly annoying on a new box.                      |
| Shared backend (Infisical) | Config declares the credential, the value resolves from Infisical after one auth |
| Encrypted file (age/sops)  | Travels in the config repo, works headless where no keychain daemon exists       |

Infisical is worth building as a first-class backend rather than a generic escape hatch, given it is already in use here for dev projects and already has a URL in the current `.env`.

### Machine-specific values, without profiles

Sync reintroduces a problem we thought we had dodged. The same `config.yaml` landing on a laptop and a headless server will reference things that exist on only one of them: a portable VS Code install, a project root, a wireguard config.

Most of this is already solved by `requires`. An entry whose binary or path is missing degrades to amber and is reported by `doctor` instead of generating something broken. No profile system needed.

For the remainder — values that genuinely differ per machine rather than merely being absent — a single unsynced `local.yaml` overlays `config.yaml`. Two files, one merge step, no profile machinery:

```
~/.config/loadout/config.yaml   synced
~/.config/loadout/local.yaml    never synced, overrides the above
```

## The `requires` mechanism

One declaration per alias and function, driving three outputs:

```yaml
pw:
  requires:
    - binary: code
    - config: workspaces # non-empty
```

- `loadout doctor` reports everything unmet.
- Calling an unconfigured command gives a real error instead of a silent failure.
- The web UI shows a green/amber/red dot per entry.

This is where the review findings become product features. On this machine right now, thirteen aliased tools are not installed, `ls` shadows a real binary, `dl` points at dead `youtube-dl`, and roughly twenty-eight aliases are Linux-only. A flat alias file cannot surface any of that.

## Web UI surface

| Section       | Manages                                                          |
| ------------- | ---------------------------------------------------------------- |
| Identities    | name, email, git user, SSH key, active identity, linked creds    |
| Credentials   | named entries per secret, which is active, keychain-backed       |
| Hosts         | alias, hostname, user, port, key or password, upload path        |
| Projects      | project root, workspaces, editors                                |
| Runtimes      | venv root, npm globals, node versions                            |
| Binaries      | installed binaries, pinned versions, updates, provenance         |
| Scripts       | builtin and user scripts on PATH, edit and enable                |
| Alias catalog | browse by category, toggle, add custom, live availability status |
| Functions     | toggle, built-in docs, `requires` status                         |
| Doctor        | missing tools, dead paths, unmet requirements, platform mismatch |
| Diff & apply  | preview the generated shell before writing it                    |

Diff-before-apply matters more than it sounds: the tool writes a file sourced into every shell you open. Showing exactly what changes is what makes that trustworthy, and it is nearly free since we are generating text anyway. A test asserts the applied file is byte-identical to what the diff previewed, because a preview that can differ from the result is worse than no preview.

### Why the UI needs real access control

Binding to 127.0.0.1 keeps other machines out. It does not keep other _pages_ out: any site open in the user's browser can issue requests to localhost. Since this API rewrites the file their shell sources, that would be a remote code execution path. Three checks, all enforced before any handler runs:

- **A per-run token**, compared in constant time. It reaches the page in the query string, because a navigation cannot set a header, and the page strips it from the URL immediately so it does not linger in the address bar or history.
- **Origin refused** unless it is our own loopback address, so another site cannot drive the API even if it somehow learned the token.
- **Host required to be loopback**, which is the defence against DNS rebinding: an attacker pointing their own hostname at 127.0.0.1 gets a Host header that does not match.

Credential values are never sent to the page. The UI is told only whether a value exists.

## Distribution

- `brew install` via a tap, for macOS and Linuxbrew.
- A `curl | sh` script fetching a release binary, for servers.
- `go install` for anyone with a Go toolchain.
- First run: `loadout init` writes a starter config and prints the one line to add to `~/.zshrc`.

## macOS parity

Not a port flag. Real behavioural differences to resolve in the catalog, roughly twenty-eight aliases:

| Linux                   | macOS                    |
| ----------------------- | ------------------------ |
| `apt-get`, `dpkg -i`    | `brew`                   |
| `systemctl` (7 aliases) | `launchctl`              |
| `netstat -tulanp`       | `lsof -i -P`             |
| `lsof -t -i:` in `kp`   | differs in flag handling |
| `wg-quick`, `openvpn3`  | different tooling        |

The catalog needs per-platform variants, not a single command string. Generating for `darwin` now yields 158 aliases against Linux's 166: `debi` and the openvpn and wireguard entries drop out entirely, and eleven resolve differently.

Tool _names_ were the easy half. The subtler risk is a command that exists on both and behaves differently. `sshp` used `find -regex`, whose flavour differs on BSD, and it matched only `*rsa` and `*.pem` — silently skipping every ed25519 key. It is now POSIX `sh` with a shell loop that identifies a private key by its content rather than its name.

One thing this audit flagged for later: `pw` uses `local -A`, an associative array. Fine for zsh, but macOS ships bash 3.2, which has no associative arrays. The bash backend will need a different implementation there.

## Distribution

Release builds are plain `go build` with `GOOS`/`GOARCH`. There is no cgo in any package, so nothing needs a build container or a release tool.

```sh
make build       # current platform
make check       # build, vet, test, format
make parity      # prove the generator still matches the original files
make release     # all four platforms into dist/ with checksums
```

Three channels:

- `install.sh`, POSIX sh so it runs on a minimal server, which verifies the published checksum before putting anything on PATH and installs via a rename so an interrupted run cannot leave a half-written executable behind. `LOADOUT_BASE_URL` points it at a mirror.
- A Homebrew formula in `packaging/homebrew`.
- `go install` for anyone with a toolchain.

## Phasing

| Phase | Deliverable                                                                             |
| ----- | --------------------------------------------------------------------------------------- |
| 0     | Go skeleton, config schema, `apply` generating zsh at parity with today's files. No UI. |
| 1     | `init` wizard and `doctor`. Identities, hosts, projects, runtimes as real config.       |
| 2     | Binaries and scripts: managed bin dir on PATH, fetch, pin, update.                      |
| 3     | Keychain credentials with active-entry switching, wired to identities.                  |
| 4     | Web UI.                                                                                 |
| 5     | Sync: git-backed config repo, `local.yaml` overlay.                                     |
| 6     | macOS parity and install channels.                                                      |
| 7     | bash backend, which validates that the backend abstraction actually holds.              |

Phase 0 is the honest test of the whole design: if the generator can reproduce the current environment exactly, everything after it is additive.

## What the bash backend proved

The backend abstraction held: bash was one new file, and every alias line the two backends emit is byte-identical. Only the preamble differs, because bash expands aliases in interactive shells only and needs `shopt -s expand_aliases` where zsh needs nothing.

The abstraction was never the risk, though. The risk was in the function bodies, and adding a second shell found four things that had been wrong all along:

| Construct                                   | What bash does                                              |
| ------------------------------------------- | ----------------------------------------------------------- |
| `${=pids}` in `kp`                          | Rejects it: bad substitution                                |
| `${env_dir:t}` in `lenv`                    | **Silently returns the whole path** instead of the basename |
| `local -A` in `pw`                          | Works on bash 4, fails on the 3.2 macOS ships               |
| `getopts` without resetting OPTIND in `msh` | Silently ignores flags on every call after the first        |

The middle two are the instructive ones. Neither is a syntax error, so the generated file passed `bash -n` cleanly while being wrong at runtime — one of them by returning a plausible wrong answer rather than failing. Templates are now written in the intersection of both dialects, with tests forbidding each construct's return.

## Open questions

1. **Alias triage.** Of the 188 aliases, some are clearly product (`gpush`, `..`, the npm and tmux sets) and some are clearly personal (`llm` pointing at `~/AnythingLLMDesktop`, `xsm:cd` with a snap path, `ovi` referencing a specific `.ovpn` file). Each needs sorting into shipped-default, user-config, or dropped.
2. **Which credential backend ships in v1.** Per-machine re-entry is the safe default and the least work. Infisical is the most useful given existing usage. Ordering is open; see Sync.
