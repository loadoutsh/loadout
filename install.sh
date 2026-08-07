#!/bin/sh
#
# loadout installer.
#
#   curl -fsSL https://raw.githubusercontent.com/wesscoby/loadout/main/install.sh | sh
#
# POSIX sh, because the shell running this is whatever the user happens to
# have, which on a minimal server may not be bash.

set -eu

REPO="wesscoby/loadout"
BINARY="loadout"
INSTALL_DIR="${LOADOUT_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${LOADOUT_VERSION:-latest}"
# Override where releases are fetched from, for a mirror, an internal
# artefact store, or testing the installer itself.
BASE_URL="${LOADOUT_BASE_URL:-}"

say() { printf '%s\n' "$*"; }
err() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

need() {
	command -v "$1" > /dev/null 2>&1 || err "$1 is required but not installed"
}

need uname
need tar

# One of these must exist to fetch anything.
if command -v curl > /dev/null 2>&1; then
	fetch() { curl -fsSL "$1"; }
	fetch_to() { curl -fsSL -o "$2" "$1"; }
elif command -v wget > /dev/null 2>&1; then
	fetch() { wget -qO- "$1"; }
	fetch_to() { wget -qO "$2" "$1"; }
else
	err "curl or wget is required"
fi

# --- platform -------------------------------------------------------------

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
	linux | darwin) ;;
	*) err "unsupported operating system: $os (loadout supports Linux and macOS)" ;;
esac

arch=$(uname -m)
case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) err "unsupported architecture: $arch" ;;
esac

say "loadout installer"
say "  platform: ${os}/${arch}"

# --- version --------------------------------------------------------------

if [ "$VERSION" = latest ] && [ -z "$BASE_URL" ]; then
	# Resolve through the redirect rather than the API, which avoids the
	# unauthenticated rate limit that bites on shared networks.
	if command -v curl > /dev/null 2>&1; then
		resolved=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
			"https://github.com/${REPO}/releases/latest" 2> /dev/null || true)
	else
		resolved=$(wget -qS --max-redirect=5 -O /dev/null \
			"https://github.com/${REPO}/releases/latest" 2>&1 |
			awk '/Location:/ {print $2}' | tail -1 || true)
	fi
	VERSION=$(printf '%s' "$resolved" | sed 's#.*/tag/##')
	[ -n "$VERSION" ] || err "could not determine the latest version; set LOADOUT_VERSION"
fi
say "  version:  $VERSION"

# --- download -------------------------------------------------------------

asset="${BINARY}_${VERSION}_${os}_${arch}.tar.gz"
if [ -n "$BASE_URL" ]; then
	base="$BASE_URL"
else
	base="https://github.com/${REPO}/releases/download/${VERSION}"
fi

tmp=$(mktemp -d)
# shellcheck disable=SC2064  # expand tmp now, not at trap time
trap "rm -rf '$tmp'" EXIT INT TERM

say "  fetching: $asset"
fetch_to "${base}/${asset}" "${tmp}/${asset}" ||
	err "download failed. Does a release named $VERSION exist for ${os}/${arch}?"

# --- verify ---------------------------------------------------------------

# This binary is about to be put on PATH and executed, so the checksum is
# verified whenever the release publishes one.
if fetch_to "${base}/checksums.txt" "${tmp}/checksums.txt" 2> /dev/null; then
	expected=$(grep " ${asset}\$" "${tmp}/checksums.txt" 2> /dev/null | awk '{print $1}' || true)
	if [ -n "$expected" ]; then
		if command -v sha256sum > /dev/null 2>&1; then
			actual=$(sha256sum "${tmp}/${asset}" | awk '{print $1}')
		elif command -v shasum > /dev/null 2>&1; then
			actual=$(shasum -a 256 "${tmp}/${asset}" | awk '{print $1}')
		else
			actual=""
			say "  checksum: skipped (no sha256 tool available)"
		fi
		if [ -n "$actual" ]; then
			[ "$actual" = "$expected" ] || err "checksum mismatch
  expected $expected
  got      $actual
Refusing to install."
			say "  checksum: verified"
		fi
	else
		say "  checksum: no entry for this asset, skipped"
	fi
else
	say "  checksum: no checksums.txt published, skipped"
fi

# --- install --------------------------------------------------------------

tar -xzf "${tmp}/${asset}" -C "$tmp"
[ -f "${tmp}/${BINARY}" ] || err "the archive did not contain a ${BINARY} binary"

mkdir -p "$INSTALL_DIR"
# Move into place with a rename so an interrupted install cannot leave a
# half-written executable on PATH.
mv "${tmp}/${BINARY}" "${INSTALL_DIR}/${BINARY}.new"
chmod 755 "${INSTALL_DIR}/${BINARY}.new"
mv "${INSTALL_DIR}/${BINARY}.new" "${INSTALL_DIR}/${BINARY}"

say ""
say "installed ${INSTALL_DIR}/${BINARY}"

case ":${PATH}:" in
	*":${INSTALL_DIR}:"*) ;;
	*)
		say ""
		say "${INSTALL_DIR} is not on your PATH. Add this to your shell config:"
		say ""
		say "  export PATH=\"\$PATH:${INSTALL_DIR}\""
		;;
esac

say ""
say "next:"
say "  loadout init      answer a few questions"
say "  loadout apply     write your shell file"
say "  loadout doctor    check it works here"
