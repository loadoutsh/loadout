# loadout

A curated shell environment, distributed as a Go CLI. It renders an opinionated
alias and function catalog against a user's configuration and writes one file
their shell sources.

## The idea that shapes everything

**An alias cannot be a program.** `gpush` and `..` are shell state, not
executables, so no binary can provide them. loadout therefore _generates shell_
rather than intercepting commands, and the binary stays out of the hot path: the
generated file is static text, regenerated only by `loadout apply`. Nothing runs
when a shell opens.

Two shipped functions, `p` and `activate`, change the calling shell's directory
and environment. They stay real shell functions because a child process cannot
`cd` for its parent. Everything else could be a subcommand, and is not, because
the generator already exists.

## Layout

```
cmd/loadout/           cobra commands, one file per command group
internal/
  config/              schema, local.yaml overlay, validation
  catalog/             the shipped set, embedded with go:embed
    data/aliases.yaml    136 aliases with category, pack, platform, requires
    data/functions.yaml  per-function metadata: summary, usage, requires
    data/functions/      text/template bodies, one per function
    data/scripts/        shipped executables (csf, sshp, tmux-session)
  generate/            rendering
    backend/           Backend interface; zsh and bash
  binaries/            fetch, verify, extract, install third-party executables
  credentials/         keychain: security (macOS), secret-tool, pass
  requires/            capability checks driving doctor, describe and the UI
  server/              localhost web UI and its access control
    web/               embedded page, no build step
  sync/                git-backed config repo, secret scanning
  selfupdate/          install-method detection, self-replacement
  uninstall/           inventory and removal
testdata/
  fixture/config.yaml  exercises every schema section
  golden/              pinned output: 2 shells x 2 platforms
packaging/homebrew/    formula template and its generator
```

## Commands

```sh
make check     # build, vet, test, format — what CI runs
make golden    # re-record golden files after an intended change
make install   # build into ~/.local/bin
make release   # cross-compile all four platforms into dist/
make tap       # render the Homebrew formula from dist/checksums.txt
make notes     # preview release notes for VERSION
```

## Invariants

Each of these was a real bug. They are not style preferences.

**A tilde does not expand inside double quotes.** Paths interpolated into
generated shell must be in `$HOME` form. The `shpath` template helper does this,
and `TestNoUnexpandedTildeInQuotes` fails on any `"~/` in non-alias output.

**Generated shell must work in both zsh and bash.** Three constructs are
forbidden and tested for:

| Never      | Because                                            | Use instead          |
| ---------- | -------------------------------------------------- | -------------------- |
| `${=var}`  | zsh-only; bash rejects it                          | `for x in $(cmd)`    |
| `${var:t}` | zsh-only; bash **silently returns the whole path** | `$(basename "$var")` |
| `local -A` | needs bash 4; macOS ships 3.2                      | a `case` statement   |

**`getopts` needs `local OPTIND=1`.** zsh localises OPTIND per function; bash
does not, so without it a second call silently ignores its flags.

**Secrets never enter a file loadout controls.** Config holds credential names
and which entry is active. Values live in the OS keychain. `loadout scan` and
`sync push` refuse anything credential-shaped.

**Anything on PATH is verified.** Downloads are HTTPS on every redirect hop,
checksums are verified before install, archive members with `..` or absolute
paths are refused, and writes land via rename so an interruption cannot leave a
half-written executable.

**The web UI is a remote code execution API for the user's account.** It rewrites
what their shell runs at login. Three checks precede every request: a per-run
token, an Origin that must be ours, and a Host that must be loopback. Static
assets are the one deliberate exception, because a `<script>` tag cannot present
a token. Do not loosen the Host check; it is the DNS rebinding defence.

**Upgrading the binary does not update anyone's shell.** `loadout apply` does.
Any change to the catalog or a template must reach users through release notes
saying so, which the release process detects from the golden files.

## Testing

**Parsing is not behaviour.** `zsh -n` and `bash -n` both pass on code that is
wrong at runtime. A formatter once rewrote an associative array key from
`[a11-wb]` to `[a11 - wb]`, leaving a function that parsed, defined, and silently
failed to look anything up. Compare what functions _do_.

**Golden files pin the whole output.** `testdata/golden` holds the complete
generated shell for both dialects on both platforms. Any catalog or template
change shows up there; re-record with `make golden` and review the diff, because
that diff is what lands in every user's shell.

**Exercise the real path.** Most bugs in this project were found by installing
and using it, not by the tests:

- `loadout env` wrote exports to stderr, so `eval "$(loadout env)"` did nothing.
  Never invoked end to end.
- The UI served no CSS or JS, because the guard required a token that a
  `<script>` tag cannot send. The test passed one by hand with curl.
- A formula generator's guard could not abort: it ran inside `$( )`, so its
  `exit` ended the subshell and the script carried on emitting a broken formula
  and exiting 0.

A check that cannot fail is worse than no check. When adding one, make it fail
first.

## Conventions

- Formatting is enforced: `shfmt` for shell, `gofmt` for Go, Prettier for
  markdown, all via `./bin/fmt`. A pre-commit hook runs it on staged files;
  enable with `git config core.hooksPath hooks`.
- Comments explain _why_, especially where the code looks odd. Most odd-looking
  code here is defending against something specific.
- Commit messages describe what changed and what it means, not just the diff.
- Nothing personal in the repo: no real hostnames, org names, paths or emails.
  The fixture and every example use illustrative values.
