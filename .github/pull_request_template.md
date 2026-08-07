## What this changes

<!-- What it does and why. The diff shows the how. -->

## Checks

- [ ] `make check` passes
- [ ] If the catalog or a template changed: `make golden` run, and
      `git diff testdata/golden` **read**, not just regenerated
- [ ] Generated shell verified in both zsh and bash if it changed
- [ ] `CHANGELOG.md` updated under `[Unreleased]` if users would notice
- [ ] No personal paths, hostnames, org names or emails, including in
      comments and fixtures

## How it was verified

<!--
Not "the tests pass". What did you actually run, and how do you know it
works? Most real bugs here passed the tests. If you added a check, did you
watch it fail first?
-->
