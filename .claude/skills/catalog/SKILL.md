---
name: catalog
description: Add, change or remove an alias, function, or pack in loadout's shipped catalog. Use whenever a change touches internal/catalog/data — aliases.yaml, functions.yaml, a .tmpl body, or a shipped script — or when deciding whether something belongs in the catalog, in user config, or in a pack.
---

# Changing the catalog

The catalog is what every user gets. A change here lands in their shell, so the
first question is never "how" but "does this belong to everyone".

## Where does it belong?

| Bucket     | Test                                                        | Where                                   |
| ---------- | ----------------------------------------------------------- | --------------------------------------- |
| **Core**   | Useful to nearly everyone, no personal values               | `data/aliases.yaml`, no `pack`          |
| **Pack**   | Tied to one tool or ecosystem                               | `data/aliases.yaml` with `pack: <name>` |
| **Config** | Contains a path, host, org or identity that varies per user | generated in `generate.go` from config  |
| **Drop**   | Personal, dead upstream, or a bad default for strangers     | nowhere                                 |

Two rules that have already been decided and should not be relitigated without
good reason:

- **Do not shadow a familiar command.** `ls`, `del`, and `ping` were all dropped
  for this. Overriding what `ls` does breaks scripts and surprises people.
- **Do not ship an alias for unmaintained software.** `youtube-dl`, `neofetch`
  and Compose v1 were dropped. If the modern equivalent is wanted, add it as a
  new entry rather than reviving the old name.

See `docs/ALIAS-TRIAGE.md` for how the original 188 were sorted.

## Adding an alias

```yaml
- name: gpush
  command: git push
  category: git
  requires: ["git"] # binaries that must exist
  pack: git-flow # omit for core
  comment: "shown beside the alias" # optional
```

When one command cannot serve both platforms, use `platforms` instead of
`command`:

```yaml
- name: ports
  platforms:
    linux: netstat -tulanp
    darwin: lsof -i -P
  category: config
```

Omit `requires` for platform-conditional entries: it is derived from the
resolved command, because the answer differs per platform. Shell builtins and
names the catalog itself provides are excluded automatically.

For Linux-only or macOS-only entries, use `platform: [linux]`.

## Adding a function

Two files, always both:

1. `data/functions/<name>.tmpl` — a `text/template` rendering to shell
2. `data/functions.yaml` — metadata

```yaml
- name: pw
  summary: One line, what it is for.
  usage: |
    pw a11         # a comment per form
  requires:
    - config:workspaces # "binary" | "path:<p>" | "config:<section>"
  config: [workspaces, editors]
  notes: |
    Anything surprising about how it behaves.
```

That metadata is not documentation for its own sake. It drives
`loadout describe`, what `doctor` reports, and the status dot in the web UI.
A function without it is invisible to all three, and
`TestEveryFunctionHasMetadata` fails.

`{{project_root}}` and `{{venv_root}}` in `requires` are substituted from the
user's paths.

### Writing the template

Read the portability rules before writing shell here: **use the
`shell-portability` skill**. The generated file has to work in zsh and bash, on
Linux and macOS, and three common constructs silently do the wrong thing.

Interpolate paths with `{{ shpath .Paths.ProjectRoot }}`, never raw. A tilde
does not expand inside double quotes.

## After any change

```sh
make golden    # re-record, then READ THE DIFF
make check
```

The golden diff is the change as users will experience it. If it is bigger than
expected, that is the finding.

When the diff is non-empty, add a line to `CHANGELOG.md` under `[Unreleased]`
saying what changed in the shell output. The release notes detect the change
automatically, but only the changelog explains it.

## Verifying

`make check` proves it compiles and parses. It does not prove the function
works. Use the `verify` skill — behaviour is the thing that has caught real bugs
here, parsing never has.
