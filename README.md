# loadout

A curated shell environment you configure rather than edit.

loadout ships an opinionated set of aliases and shell functions, renders them against your own configuration, and writes a single file your shell sources at startup. The personal parts — who you are in git, where your projects live, which machines you upload to — are configuration, not something you fork and hand-edit.

Linux and macOS. zsh and bash.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/wesscoby/loadout/main/install.sh | sh
```

Or with Homebrew:

```sh
brew tap wesscoby/loadout
brew install loadout
```

Or from source, if you have Go:

```sh
go install github.com/wesscoby/loadout/cmd/loadout@latest
```

## Getting started

```sh
loadout init      # a few questions; detected values offered as defaults
loadout apply     # render your configuration into a shell file
loadout doctor    # check everything it asks for works on this machine
```

Then add one line to your shell config:

```sh
source ~/.config/loadout/init.zsh
```

loadout never edits your shell config for you. That line is yours to add.

## How it works

```
~/.config/loadout/config.yaml  ──►  loadout apply  ──►  ~/.config/loadout/init.zsh
        ▲                                                        │
        │                                                        ▼
   loadout CLI  ◄──►  loadout ui                        sourced by your shell
```

An alias cannot be a program: `gpush` and `..` are shell state, so loadout generates shell rather than intercepting commands. Nothing runs when you open a terminal — the generated file is static text, regenerated only when you run `apply`.

Two of the shipped functions, `p` and `activate`, change your shell's directory and environment. They stay real shell functions for that reason; a child process cannot `cd` for its parent.

## Commands

| Command            | What it does                                                     |
| ------------------ | ---------------------------------------------------------------- |
| `loadout init`     | Create your configuration                                        |
| `loadout apply`    | Render the shell file                                            |
| `loadout doctor`   | Report anything that would not work here                         |
| `loadout describe` | What a command does, how to call it, what it needs               |
| `loadout catalog`  | What the shipped set contains                                    |
| `loadout ui`       | Local web interface                                              |
| `loadout bin`      | Manage third-party executables on your PATH                      |
| `loadout cred`     | Secrets, stored in your OS keychain                              |
| `loadout env`      | Export the active credentials into your shell                    |
| `loadout identity` | Switch git author, SSH key and credentials together              |
| `loadout sync`     | Keep your configuration in git across machines                   |
| `loadout scan`     | Check your config for anything that looks like a live credential |

## Configuration

Everything personal lives in `~/.config/loadout/config.yaml`.

```yaml
paths:
  project_root: ~/Projects
  venv_root: ~/.venv

identities:
  - id: personal
    git_user: Your Name
    email: you@example.com
    ssh_key: ~/.ssh/id_ed25519
    default: true

workspaces:
  - name: api
    path: ~/Projects/api/api.code-workspace
    tmux_session: api

hosts:
  - alias: staging
    hostname: staging.example.com
    user: deploy
    key: ~/.ssh/deploy

packs: [ansible, cloudflared]
```

Run `loadout describe <command>` to see which sections shape which command.

### Packs

Most of the catalog is on by default. Ecosystem-specific bundles — flutter, jekyll, gatsby, exercism, git-flow, cloudflared, ansible — are packs, off unless you enable them. Toggle them in `loadout ui` or list them under `packs:`.

### Anything you disagree with

```yaml
aliases:
  disabled: [ls, py]
  custom:
    - name: deploy
      command: ./scripts/deploy.sh
      platform: [linux]
```

## Credentials

Secrets live in your OS keychain — Keychain on macOS, libsecret or `pass` on Linux — and never in a file loadout controls. Your configuration records only which entries exist and which is active.

```sh
loadout cred set npm_token work    # prompts, does not echo
loadout cred use npm_token work    # switch the active entry
eval "$(loadout env)"              # bring the active values into this shell
```

That `eval` is a deliberate cost. Baking secrets into the generated file would put them in plain text on disk, so something has to run to fetch them. loadout does not add it to your shell startup; do that yourself only if you want the values in every shell.

An identity can nominate credential entries, so `s work` switches your git author, your SSH key and your tokens in one command.

## Binaries and scripts

Third-party executables are declared, not committed, and fetched per platform into a managed directory that loadout appends to your `PATH`.

```yaml
binaries:
  - name: duckdb
    version: v1.1.3
    source:
      github: duckdb/duckdb
      asset:
        linux-amd64: duckdb_cli-linux-amd64.zip
        darwin-arm64: duckdb_cli-osx-universal.zip
    checksum:
      linux-amd64: sha256:efd0fcc...
```

```sh
loadout bin install   # fetch, verify, install
loadout bin list      # versions, source URLs, whether verified
```

This is the one part that downloads and then runs code, so: HTTPS on every redirect hop, checksums verified before anything is installed, archive members with unsafe paths refused, and the source URL recorded so you can always see where something on your PATH came from.

The managed directory is **appended** to `PATH`, not prepended, so a managed binary cannot shadow a system tool.

## Sync

```sh
loadout sync init git@github.com:you/loadout-config.git
loadout sync push
loadout sync pull
```

Only `config.yaml` travels. The generated file is regenerated per machine, and `local.yaml` exists to hold whatever must not travel — it overlays `config.yaml` and is never synced.

A scanner runs before the commit, not before the push, so a credential pasted into your config never enters the history at all.

Use a private repository. The config holds no secrets, but it names your hosts, paths and identities.

## Development

```sh
make check     # build, vet, test, format
make golden    # re-record the golden files after an intended change
make release   # cross-compile every platform into dist/ with checksums
```

The generated shell is pinned by golden files in `testdata/golden`, covering both shells and both platforms. A change to the catalog or a template shows up as a diff there, which is what you review.

## Licence

MIT. See [LICENSE](LICENSE).
