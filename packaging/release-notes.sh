#!/usr/bin/env bash

#: release-notes.sh -- Build the body of a GitHub release.
#:
#: Combines the hand-written CHANGELOG section for this version with the things
#: nobody should have to remember: whether the generated shell changed, how to
#: install and upgrade, and the checksums.
#:
#: Usage: release-notes.sh <tag> [previous-tag]
#:
#: The previous tag is worked out from the repository when not given.

set -euo pipefail

TAG="${1:?usage: release-notes.sh <tag> [previous-tag]}"
VERSION="${TAG#v}"
CHANGELOG="${CHANGELOG:-CHANGELOG.md}"
GOLDEN_DIR="${GOLDEN_DIR:-testdata/golden}"

# The tag being released already exists when this runs in CI, so exclude it
# when looking for what came before.
PREV="${2:-}"
if [[ -z "$PREV" ]]; then
	PREV=$(git tag --sort=-v:refname | grep -v "^${TAG}$" | head -1 || true)
fi

# --- the hand-written part -------------------------------------------------

# Pull out the section for this version: everything between its heading and the
# next one. Falls back to Unreleased, which is where notes usually sit right up
# until the tag is cut.
extract_section() {
	local heading="$1"
	awk -v want="$heading" '
		$0 ~ "^## \\[" want "\\]" { grabbing = 1; next }
		grabbing && /^## / { exit }
		grabbing { print }
	' "$CHANGELOG"
}

body=""
if [[ -f "$CHANGELOG" ]]; then
	body=$(extract_section "$VERSION")
	[[ -z "${body//[[:space:]]/}" ]] && body=$(extract_section "Unreleased")
fi

if [[ -z "${body//[[:space:]]/}" ]]; then
	body="See the commit log for what changed in this release."
fi

printf '%s\n' "${body#"${body%%[![:space:]]*}"}"

# --- does this release change anyone's shell? ------------------------------
#
# Upgrading the binary does not rewrite the file a shell sources; apply does.
# Someone who upgrades and does not re-apply keeps the old aliases and will not
# know why. The golden files are committed, so this is answerable rather than
# something a release manager has to remember.

have_rev() { git rev-parse -q --verify "${1}^{commit}" > /dev/null 2>&1; }

# When run before the tag exists, which is what `make notes` does, compare
# against the working revision so a preview is still useful.
target="$TAG"
have_rev "$target" || target="HEAD"

# Only claim anything when the comparison can actually be made. A failed diff
# must not be read as "changed": that would announce shell changes on releases
# that have none, and the announcement is the thing people act on.
if [[ -n "$PREV" ]] && have_rev "$PREV" && have_rev "$target"; then
	if ! git diff --quiet "$PREV" "$target" -- "$GOLDEN_DIR"; then
		diff_out=$(git diff "$PREV" "$target" -- "$GOLDEN_DIR")
		added=$(grep -c '^+alias ' <<< "$diff_out" || true)
		removed=$(grep -c '^-alias ' <<< "$diff_out" || true)

		# Counts only when they are meaningful. A function body can change with
		# no alias lines touched, and "0 added, 0 removed" printed next to "the
		# shell changed" just reads as a bug.
		detail=""
		if ((added > 0 || removed > 0)); then
			detail=$(printf 'Across the four generated files, %d alias line(s) were added and %d removed.' \
				"$added" "$removed")
		fi

		cat << EOF

## Run \`loadout apply\` after upgrading

The generated shell changed in this release. Upgrading the binary does not
rewrite the file your shell sources, so your aliases and functions stay as they
were until you run:

\`\`\`sh
loadout apply
\`\`\`

${detail}
Nothing is written until you run it, and \`loadout ui\` shows the diff first.
EOF
	fi
fi

# --- how to get it ---------------------------------------------------------

cat << EOF

## Installing

\`\`\`sh
curl -fsSL https://raw.githubusercontent.com/loadoutsh/loadout/main/install.sh | sh
brew install loadoutsh/loadout/loadout
go install github.com/loadoutsh/loadout/cmd/loadout@${TAG}
\`\`\`

## Upgrading

\`\`\`sh
loadout update          # installed via the script above
brew upgrade loadout    # installed via Homebrew
\`\`\`

\`loadout update\` refuses to replace a binary Homebrew or the Go toolchain owns,
and prints the right command instead.
EOF

# --- verification ----------------------------------------------------------

if [[ -f dist/checksums.txt ]]; then
	cat << EOF

## Checksums

Verified automatically by \`install.sh\` and \`loadout update\`. To check by hand:

\`\`\`
$(cat dist/checksums.txt)
\`\`\`
EOF
fi

if [[ -n "$PREV" ]]; then
	printf '\n**Full diff**: https://github.com/loadoutsh/loadout/compare/%s...%s\n' "$PREV" "$TAG"
fi
