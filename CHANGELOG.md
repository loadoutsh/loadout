# Changelog

Notable changes, newest first. Loosely follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
[semantic versioning](https://semver.org/spec/v2.0.0.html).

The section for a version becomes the body of its GitHub release, so write it
for someone deciding whether to upgrade rather than for the commit log.

**Anything that changes the generated shell belongs under "Shell output".**
Upgrading the binary does not rewrite the file your shell sources: `loadout apply`
does. The release process detects those changes from the golden files and says
so on the release page, but calling them out here is what tells people why.

## [Unreleased]

## [0.2.0] - 2026-08-07

### Added

- `env:` on a credential, setting the environment variable it exports when the
  key does not derive the right name. `gh_token` cannot produce `GHP_TOKEN`, and
  before this there was no way to say so.
- A README section on the web interface: why it binds to loopback, how to reach
  it from a headless server over SSH forwarding, and why a reverse proxy is
  refused.
- This changelog, and release notes built from it.
- Contribution docs: `CONTRIBUTING.md`, `SECURITY.md`, a code of conduct, and
  issue and pull request templates.
- `CLAUDE.md` and four project skills under `.claude/skills`, covering the
  catalog, shell portability, verification and the release process.

### Changed

- Two credentials resolving to the same environment variable is now rejected at
  load time. Previously one silently overwrote the other, and which one won
  depended on map iteration order.
- Examples and test fixtures use generic names throughout.

## [0.1.0] - 2026-08-07

First release.

### Added

- Generates a single shell file from a configuration file, for zsh and bash, on
  Linux and macOS. 136 curated aliases, 34 of them in seven opt-in packs, and 14
  shell functions.
- `doctor`, reporting anything that would not work on this machine: missing
  tools, dead paths, unmet requirements, output that has fallen behind the
  config.
- `describe`, explaining what a command does, how to call it, what configures
  it, and whether its prerequisites are met.
- `ui`, a local web interface on loopback with a per-run token, Origin and Host
  checks, and a diff of the generated shell before anything is written.
- Managed binaries: third-party executables declared in config and fetched per
  platform, verified against a checksum, into a directory appended to `PATH`.
- Credentials in the OS keychain, with several named entries per secret and one
  active. Switching identity switches git author, SSH key and tokens together.
- `sync`, keeping the configuration in git across machines, refusing to commit
  anything that looks like a live credential.
- `update` and `uninstall`, both aware of how loadout was installed.

[unreleased]: https://github.com/loadoutsh/loadout/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/loadoutsh/loadout/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/loadoutsh/loadout/releases/tag/v0.1.0
