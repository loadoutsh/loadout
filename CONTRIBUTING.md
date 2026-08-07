# Contributing

Thanks for looking. This document covers what the project will and will not
take, and the few rules that exist because something went wrong without them.

## Before a large change

Open an issue first. loadout is opinionated on purpose, and the most common
reason to decline a pull request is that the change belongs somewhere other than
the shipped catalog — not that it is wrong.

Small fixes, typos, and clearly-scoped bugs need no discussion. Send them.

## Getting set up

Requires Go (see `go.mod`), plus `shfmt` and `prettier` for the format check.

```sh
git clone https://github.com/loadoutsh/loadout.git
cd loadout
git config core.hooksPath hooks    # formats staged files on commit
make check                         # what CI runs
```

Test against a scratch config rather than your own:

```sh
export XDG_CONFIG_HOME=/tmp/lo-test
mkdir -p $XDG_CONFIG_HOME/loadout
cp testdata/fixture/config.yaml $XDG_CONFIG_HOME/loadout/
make install && loadout apply
```

Note that the OS keychain is **not** sandboxable by `XDG_CONFIG_HOME`. There is
one per user. Anything touching credentials should use `credentials.NewFake()`,
or sit behind `LOADOUT_KEYRING_TEST=1` and clean up after itself.

## Where a change belongs

| Bucket     | Test                                                        |
| ---------- | ----------------------------------------------------------- |
| **Core**   | Useful to nearly everyone, contains no personal values      |
| **Pack**   | Tied to one tool or ecosystem, off by default               |
| **Config** | Contains a path, host, org or identity that varies per user |
| **Drop**   | Personal, dead upstream, or a bad default for strangers     |

Two decisions already made, which need a strong argument to revisit:

- **Nothing shadows a familiar command.** `ls`, `del` and `ping` were dropped
  for this. Changing what `ls` does breaks scripts and surprises people.
- **Nothing ships an alias for unmaintained software.** If a modern equivalent
  is wanted, add it under a new name.

`docs/ALIAS-TRIAGE.md` records how the original set was sorted, and
`docs/DESIGN.md` explains the architecture and the decisions behind it.

## Rules that exist for a reason

Each of these was a real bug.

**Generated shell must work in zsh and bash, on Linux and macOS.** Three
constructs are banned and tested for: `${=var}` (bash rejects it), `${var:t}`
(bash silently returns the whole path), and `local -A` (needs bash 4; macOS
ships 3.2). `getopts` needs an explicit `local OPTIND=1`, because zsh localises
it and bash does not.

**A tilde does not expand inside double quotes.** Interpolate paths with the
`shpath` template helper.

**Secrets never go in a file loadout controls.** Config holds names and which
entry is active; values live in the keychain.

**Anything that lands on PATH is verified.** HTTPS on every redirect hop,
checksum before install, no archive members with `..` or absolute paths, writes
by rename.

**Do not loosen the web UI's Host check.** It is the DNS rebinding defence on an
API that rewrites what a shell executes at login.

## Testing

`make check` is necessary and not sufficient. Nearly every real bug in this
project passed the tests and failed in use:

- a function that parsed, defined, and silently looked nothing up
- `loadout env` writing exports to stderr, so `eval "$(loadout env)"` did nothing
- a page with no CSS, because the test passed a token a `<script>` tag cannot
- a guard that could not abort, because it ran inside `$( )`

So:

- **Compare behaviour, not parsing.** `zsh -n` and `bash -n` pass on all of the
  above.
- **Make a new check fail before trusting it.** Break the thing deliberately and
  confirm the check catches it.
- **Exercise the path a real user takes.** `curl -H` supplying a token is not
  how a browser fetches a stylesheet.

After any catalog or template change:

```sh
make golden
git diff testdata/golden    # this is what lands in every user's shell — read it
```

## Commits and pull requests

Formatting is enforced by `./bin/fmt`; the pre-commit hook handles it.

Write commit messages that explain what changed and why it matters, not what the
diff already shows. Where the code looks odd, say what it is defending against —
most odd-looking code here is defending against something specific.

Add a line to `CHANGELOG.md` under `[Unreleased]` when users would notice the
change. If the golden files moved, say so there: the release notes detect it
automatically, but only the changelog explains it.

Keep nothing personal in the repository. No real hostnames, org names, home
paths or email addresses, including in examples, comments and test fixtures.
This has been cleaned up twice already.

## Reporting bugs

Include your OS and shell, `loadout version`, and the output of
`loadout doctor`. For anything about generated shell, the relevant section of
`~/.config/loadout/init.zsh` helps — but read it first, since it reflects your
configuration.

**Do not paste your config or `loadout env` output without checking it.** Run
`loadout scan` first.

Security issues go to [SECURITY.md](SECURITY.md), not the issue tracker.

## Code of conduct

By participating you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).
