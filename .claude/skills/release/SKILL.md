---
name: release
description: Cut a loadout release, or diagnose one that went wrong. Use when tagging a version, preparing the changelog for a release, publishing to the Homebrew tap, or when install.sh, loadout update, or brew are not seeing a new version.
---

# Releasing

Pushing a `v*` tag runs `.github/workflows/release.yml`, which tests,
cross-compiles four platforms, builds the notes, publishes the release, renders
the Homebrew formula and commits it to the tap. The work is in what happens
before the tag.

## Before tagging

**1. Move the changelog section.** Rename `## [Unreleased]` to
`## [X.Y.Z] - YYYY-MM-DD`, add a fresh empty `[Unreleased]` above it, and update
the link definitions at the bottom. That section becomes the release body, so
write it for someone deciding whether to upgrade.

**2. Check whether the shell output changed.**

```sh
git diff v<previous> HEAD -- testdata/golden
```

Non-empty means every user must run `loadout apply` after upgrading, or their
shell keeps the old aliases and they will not know why. The notes detect this
automatically, but the changelog is what explains it.

**3. Preview the notes.**

```sh
make notes VERSION=vX.Y.Z
```

Read them. Confirm the "run `loadout apply`" section appears if and only if the
golden files changed.

**4. Green gate.**

```sh
make check
make release VERSION=vX.Y.Z    # all four platforms build
make tap VERSION=vX.Y.Z        # formula renders and is valid ruby
make clean
```

## Tagging

```sh
git push origin main           # let CI go green first
git tag -a vX.Y.Z -m "loadout vX.Y.Z"
git push origin vX.Y.Z
```

CI on `main` runs the macOS job. Tests here have a history of passing on Linux
and mattering on macOS, so wait for it before tagging.

## Versioning

`vMAJOR.MINOR.PATCH`, and the comparison in `selfupdate` accepts nothing else —
a pre-release suffix or a commit sha reports as "not comparable" rather than
being coerced into a wrong ordering.

- **Patch**: fixes, no change to generated shell or config schema
- **Minor**: new aliases, functions, packs, commands, or config fields
- **Major**: a config schema change users must act on, or a removal

## Afterwards

```sh
curl -fsSL https://raw.githubusercontent.com/loadoutsh/loadout/main/install.sh | sh
loadout version
loadout update --check
brew upgrade loadout
```

Verify the tap formula's checksums match the published release. The workflow
does this correctly; a mismatch means something republished.

## When something is wrong

**`install.sh` says it cannot determine the latest version.** No published
release, or the tag has no GitHub Release attached. `install.sh` resolves
`latest` through the `/releases/latest` redirect, so a bare tag is not enough.

**brew still installs the old version.** The tap was not updated. Check the
workflow's tap step: without `TAP_GITHUB_TOKEN` it warns rather than fails, on
purpose, so a missing tap does not block the release everyone else uses. Publish
`dist/loadout.rb` by hand in that case.

**`loadout update` says "not a release version".** The running build is a
development build, or the server returned something unparseable. The message
distinguishes the two — read which side it names.

**The formula fails to install.** Its checksums must match the _published_
artefacts, which CI built. Locally built tarballs will not have the same hashes.

## Rewriting a published tag

Possible, and occasionally right while the project is young: force-push the tag
and the release survives with its assets. Two things to know. The tag must move
or the old commit stays reachable, defeating the point. And the published
binaries were built from the old commit, so confirm nothing that reaches the
binary actually changed — comments, docs and tests do not.
