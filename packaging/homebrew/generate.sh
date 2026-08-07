#!/usr/bin/env bash

#: generate.sh -- Render the Homebrew formula for a release.
#:
#: Reads dist/checksums.txt produced by `make release` and fills in the
#: template. Every platform must be present: a formula with an empty sha256
#: does not fail to build, it installs and then mismatches, and the error a
#: user sees points at the wrong thing entirely.
#:
#: Usage: generate.sh <tag> [checksums-file] [template]

set -euo pipefail

TAG="${1:?usage: generate.sh <tag> [checksums] [template]}"
CHECKSUMS="${2:-dist/checksums.txt}"
TEMPLATE="${3:-packaging/homebrew/loadout.rb.tmpl}"

[[ -f "$CHECKSUMS" ]] || {
	echo "no checksums at $CHECKSUMS; run 'make release' first" >&2
	exit 1
}
[[ -f "$TEMPLATE" ]] || {
	echo "no template at $TEMPLATE" >&2
	exit 1
}

# Homebrew's version field is the bare number; the URLs carry the tag, which
# conventionally has a leading v. Keeping them separate avoids a formula whose
# version and download path disagree.
VERSION="${TAG#v}"

# sha_for only prints. It must not try to abort: called from inside a command
# substitution, an exit here would end that subshell and leave the caller
# carrying on with an empty string. Validation happens in the main shell below.
sha_for() {
	local asset="loadout_${TAG}_${1}.tar.gz"
	awk -v want="$asset" '$2 == want { print $1 }' "$CHECKSUMS"
}

PLATFORMS=(darwin_arm64 darwin_amd64 linux_arm64 linux_amd64)

declare -A SHA
missing=()
for platform in "${PLATFORMS[@]}"; do
	# Assigned in the main shell, so a failure below can actually stop the run.
	SHA[$platform]=$(sha_for "$platform")
	if [[ -z "${SHA[$platform]}" ]]; then
		missing+=("loadout_${TAG}_${platform}.tar.gz")
	fi
done

if ((${#missing[@]} > 0)); then
	echo "cannot render the formula: no checksum for" >&2
	printf '  %s\n' "${missing[@]}" >&2
	echo >&2
	echo "The release must build every platform the formula declares." >&2
	echo "Run 'make release' and check dist/checksums.txt." >&2
	exit 1
fi

formula=$(sed \
	-e "s|{{TAG}}|${TAG}|g" \
	-e "s|{{VERSION}}|${VERSION}|g" \
	-e "s|{{SHA_DARWIN_ARM64}}|${SHA[darwin_arm64]}|g" \
	-e "s|{{SHA_DARWIN_AMD64}}|${SHA[darwin_amd64]}|g" \
	-e "s|{{SHA_LINUX_ARM64}}|${SHA[linux_arm64]}|g" \
	-e "s|{{SHA_LINUX_AMD64}}|${SHA[linux_amd64]}|g" \
	"$TEMPLATE")

# A leftover placeholder means the template grew a field the generator does
# not know about.
if grep -q '{{' <<< "$formula"; then
	echo "unsubstituted placeholder left in the formula:" >&2
	grep -n '{{' <<< "$formula" >&2
	exit 1
fi

# An empty sha256 would install and then mismatch. Refuse rather than publish.
if grep -qE 'sha256 ""' <<< "$formula"; then
	echo "the rendered formula contains an empty sha256" >&2
	exit 1
fi

printf '%s\n' "$formula"
