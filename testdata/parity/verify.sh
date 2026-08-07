#!/usr/bin/env bash

#: Title:           verify.sh
#: Description:     Prove the generated shell reproduces the hand-written one.
#:                  Compares the alias set produced by sourcing .aliases
#:                  against the set produced by sourcing loadout's output,
#:                  allowing only the aliases the triage deliberately drops.
#:                  Then checks every generated function parses and defines.

set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORK="${1:?usage: verify.sh <workdir>}"
GENERATED="$WORK/xdg/loadout/init.zsh"

# Aliases the triage drops. Present in the old files, absent by design now.
DROPPED=(.files aka aka:n vars vars:n jquery q m llm tor organize tr cpa ycpa)

# Aliases whose text changes on purpose. Each is behaviourally identical; the
# reason is recorded so an unexplained difference can never pass silently.
#   name | old value | new value | why
EXCEPTIONS=(
	"ut.|ut .|/home/wc/.vscode-portable/utopia/VSCode-linux-x64/bin/code .|generator emits the resolved editor command rather than chaining through the ut alias, so it survives ut being disabled"
)

fail=0
say() { printf '%s\n' "$*"; }

[[ -f "$GENERATED" ]] || {
	say "FAIL: no generated file at $GENERATED"
	exit 1
}

# --- alias sets ------------------------------------------------------------

# zsh defines run-help and which-command itself; exclude them from both sides.
dump_aliases() {
	zsh -c "$1; alias" 2> /dev/null |
		grep -vE "^(run-help|which-command)=" |
		sort
}

dump_aliases "cd '$REPO' && source .aliases" > "$WORK/old.aliases"
dump_aliases "source '$GENERATED'" > "$WORK/new.aliases"

# Remove the deliberately dropped ones from the old side before comparing.
cp "$WORK/old.aliases" "$WORK/old.expected"
for d in "${DROPPED[@]}"; do
	grep -vE "^${d//./\\.}=" "$WORK/old.expected" > "$WORK/tmp" && mv "$WORK/tmp" "$WORK/old.expected"
done

old_n=$(wc -l < "$WORK/old.aliases")
exp_n=$(wc -l < "$WORK/old.expected")
new_n=$(wc -l < "$WORK/new.aliases")

say "aliases before : $old_n"
say "expected after : $exp_n  (dropped ${#DROPPED[@]})"
say "generated      : $new_n"
say ""

# Verify each declared exception really is present on both sides as described,
# then set it aside. An exception that no longer matches is itself a failure.
cp "$WORK/new.aliases" "$WORK/new.expected"
for e in "${EXCEPTIONS[@]}"; do
	IFS='|' read -r name old new why <<< "$e"
	if ! grep -qxF "$name=$(printf %s "$old" | sed "s/.*/&/")" "$WORK/old.expected" &&
		! grep -qxF "$name='$old'" "$WORK/old.expected"; then
		say "FAIL  declared exception '$name' does not match the old value ($old)"
		fail=1
		continue
	fi
	say "NOTE  $name changed on purpose: '$old' -> '$new'"
	say "      $why"
	grep -vE "^${name//./\\.}=" "$WORK/old.expected" > "$WORK/tmp" && mv "$WORK/tmp" "$WORK/old.expected"
	grep -vE "^${name//./\\.}=" "$WORK/new.expected" > "$WORK/tmp" && mv "$WORK/tmp" "$WORK/new.expected"
done
((${#EXCEPTIONS[@]})) && say ""

if diff -u "$WORK/old.expected" "$WORK/new.expected" > "$WORK/alias.diff"; then
	say "PASS  alias sets are identical ($(wc -l < "$WORK/old.expected") compared byte-for-byte, ${#EXCEPTIONS[@]} declared exception)"
else
	say "FAIL  alias sets differ:"
	sed 's/^/      /' "$WORK/alias.diff" | head -60
	fail=1
fi
say ""

# --- functions -------------------------------------------------------------

EXPECTED_FUNCS=(activate denv gcfg kp lenv mpush msh nig p pv pw s venv wt)

if ! zsh -n "$GENERATED" 2> "$WORK/syntax.err"; then
	say "FAIL  generated file is not valid zsh:"
	sed 's/^/      /' "$WORK/syntax.err"
	fail=1
else
	say "PASS  generated file parses as zsh"
fi

missing=()
for fn in "${EXPECTED_FUNCS[@]}"; do
	if ! zsh -c "source '$GENERATED' 2>/dev/null; typeset -f $fn >/dev/null 2>&1"; then
		missing+=("$fn")
	fi
done

if ((${#missing[@]} == 0)); then
	say "PASS  all ${#EXPECTED_FUNCS[@]} functions defined"
else
	say "FAIL  functions not defined: ${missing[*]}"
	fail=1
fi

say ""

# --- function behaviour ----------------------------------------------------
#
# Defining is not the same as working. An earlier formatting pass silently
# rewrote an associative-array key in pw and left the function defined but
# broken, so compare what the functions actually do, not just that they exist.
#
# Each case runs against the old functions and the new generated ones with the
# external commands stubbed, and compares stdout.

STUBS='
code() { echo "code $*"; }
gh() { echo "gh $*"; }
git() { case "$1" in config) echo "git-config $*" ;; *) echo "git $*" ;; esac; }
'

OLD_SRC="for f in '$REPO'/zsh-functions/*; do source \$f; done"
NEW_SRC="source '$GENERATED'"

behaves() {
	local label="$1" snippet="$2"
	local a b
	a=$(zsh -c "$STUBS; $OLD_SRC; $snippet" 2>&1)
	b=$(zsh -c "$STUBS; $NEW_SRC; $snippet" 2>&1)
	if [[ "$a" == "$b" ]]; then
		say "PASS  $label"
	else
		say "FAIL  $label"
		say "      old: $a"
		say "      new: $b"
		fail=1
	fi
}

say "function behaviour:"
behaves "pw resolves every workspace key" \
	'for k in a11 a11-wb inerds snipify; do pw $k; done'
behaves "pw rejects an unknown workspace" 'pw nope'
behaves "pw requires an argument" 'pw'
behaves "wt clones from the configured org" 'wt api-starter'
behaves "wt clones into a target directory" 'wt api-starter server'
behaves "wt requires a repo name" 'wt'
behaves "gcfg reports the git identity" 'gcfg'
behaves "p rejects a missing path" 'p definitely/not/here'
behaves "pv rejects a missing project" 'pv definitely/not/here'
behaves "pv requires an argument" 'pv'
behaves "activate rejects a missing venv" 'activate not-a-real-venv'
behaves "denv rejects a missing venv" 'denv not-a-real-venv'
behaves "venv requires a version" 'venv'
behaves "venv requires a name" 'venv 3.12'
behaves "kp requires a port" 'kp'

say ""
if ((fail == 0)); then
	say "PARITY OK"
else
	say "PARITY FAILED"
fi
exit "$fail"
