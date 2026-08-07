# Security

## Reporting a vulnerability

Report privately through
[GitHub's advisory form](https://github.com/loadoutsh/loadout/security/advisories/new),
not the public issue tracker.

Please include what an attacker gains, the steps to reproduce, and the version
and platform. You should get an acknowledgement within a few days. This is a
small project with one maintainer, so a fix may take longer than that; you will
be told either way.

## What loadout can do, and therefore what matters

Three parts of loadout are worth attacking, because each can end in code
execution as the user.

**It writes a file your shell sources at every login.** Anything that can change
`~/.config/loadout/init.zsh`, or change the config it is generated from, can run
arbitrary code as you the next time you open a terminal. This is the most
valuable target in the project.

**It downloads and installs executables onto your PATH.** `loadout bin install`
and `loadout update` fetch third-party artefacts and make them runnable.

**It reads secrets out of your OS keychain.** `loadout env` and identity
switching resolve real credentials.

## The defences

Understanding these is usually the fastest route to finding a hole in one.

### The web interface

Binding to `127.0.0.1` keeps other machines out. It does **not** keep other
pages out: any site open in your browser can issue requests to localhost. Three
checks run before any handler.

- **A per-run token**, compared in constant time, generated at startup and
  invalid once the process ends. It reaches the page in the query string,
  because a navigation cannot set a header, and the page moves it into
  `sessionStorage` and strips it from the URL.
- **Origin** must be our own loopback address, or absent.
- **Host** must be loopback with the right port. This is the DNS rebinding
  defence: an attacker pointing a hostname they control at `127.0.0.1` gets a
  Host that will not match.

Static assets (`app.css`, `app.js`) are deliberately served without a token,
because a `<link>` or `<script>` tag cannot present one. They are inert files
compiled into the binary and identical for every user. Everything that reads
configuration or writes to disk is behind the token.

**There is no reverse proxy support, on purpose.** See the README for the
reasoning. If you are running this behind a proxy, the authentication has to be
in front of it.

### Downloads

- HTTPS on every hop, **including redirects**. Release hosts redirect to CDNs,
  and checking only the configured URL is not enough.
- Declared checksums are verified before anything is installed. `loadout update`
  refuses a release with no published checksum outright.
- Archive members with absolute or `..` paths are refused.
- Downloads and extraction are size-capped.
- Writes go to a temporary file and are renamed into place, so an interrupted
  install cannot leave a half-written executable on PATH.
- The managed bin directory is **appended** to PATH, never prepended, so a
  managed binary cannot shadow a system tool.
- The source URL of everything installed is recorded, and `loadout bin list`
  shows it along with whether the checksum was verified.

### Secrets

- Values live in the OS keychain (`security`, `secret-tool`, or `pass`) and
  never in a file loadout controls.
- Config holds only entry names and which one is active.
- `loadout env` single-quotes every value with embedded quotes escaped, so a
  token containing `$(...)` cannot execute anything when the output is eval'd.
- The web UI is told only _whether_ a value exists, never the value.
- `loadout sync` scans before committing, not before pushing, so a credential
  pasted into config never enters git history.
- `loadout uninstall` lists keychain entries by name and requires a separate
  confirmation before deleting any.

## Out of scope

- **Anything requiring local access to an already-compromised account.** If an
  attacker can write to your `~/.config`, they can write to your `.zshrc`
  directly.
- **The contents of your own config.** loadout runs the commands you configure.
  An alias you added that does something dangerous is not a vulnerability.
- **Third-party binaries you declare.** loadout verifies the download matches
  the checksum you pinned; it does not vet what the software does.
- **Missing hardening on a machine you have deliberately opened up**, for
  example by putting the UI behind a proxy against the documented advice.

## Supported versions

Only the latest release. This project is young; upgrade before reporting.
