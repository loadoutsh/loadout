---
name: shell-portability
description: Rules for writing shell that loadout generates, so it behaves identically in zsh and bash on Linux and macOS. Use when editing anything under internal/catalog/data/functions or data/scripts, when adding a backend, or when a generated function works in one shell and not the other.
---

# Writing portable generated shell

Everything loadout generates must behave identically in zsh and bash, on Linux
and macOS. The dangerous cases are not syntax errors — those get caught. They
are constructs that parse fine in both and mean different things.

## The forbidden three

Each was shipped, each was wrong, each is now covered by a test.

### `${=var}` — zsh-only word splitting

bash rejects it outright: `bad substitution`. Loud, at least.

```sh
# no
sudo kill -9 ${=pids}

# yes — command substitution word-splits in a for loop in both shells
for pid in $(sudo lsof -t -i:"$1"); do
    sudo kill -9 "$pid"
done
```

### `${var:t}` — zsh tail modifier

The worst of the three. bash does **not** error; it returns the whole path. A
plausible wrong answer, silently.

```sh
# no
echo "${env_dir:t}"

# yes
echo "$(basename "$env_dir")"
```

### `local -A` — associative arrays

Needs bash 4. macOS ships bash 3.2 and always will, for licensing reasons.

```sh
# no
local -A workspaces=(["a11-wb"]="...")

# yes
local target=""
case "$1" in
    a11-wb) target="..." ;;
esac
```

The `case` form is better anyway. The array version was also the source of a
separate bug: `shfmt` reads an unquoted subscript as arithmetic and rewrote
`[a11-wb]` into `[a11 - wb]`, producing a function that parsed, defined, and
silently looked nothing up. There is no subscript to misread in a `case`.

## Two more that bite

**`getopts` needs `local OPTIND=1`.** zsh makes OPTIND local to a function
automatically; bash does not. Without the reset, a second call in the same shell
silently ignores its flags and uses defaults.

**A tilde does not expand inside double quotes.** `[[ -e "~/Projects/$1" ]]` is
always false. Interpolate paths with the `shpath` template helper, which emits
`$HOME` form, safe both quoted and bare.

## GNU versus BSD

macOS ships BSD userland. Anything shipped in `data/scripts` runs there.

| Avoid         | Why                              |
| ------------- | -------------------------------- |
| `find -regex` | different flavour on BSD         |
| `sed -i`      | BSD requires an argument to `-i` |
| `readlink -f` | not on older macOS               |
| `date -d`     | GNU only                         |
| `stat -c`     | GNU only                         |
| `grep -P`     | not on BSD                       |

Prefer a POSIX shell loop over a clever `find`. `sshp` was rewritten that way,
which also fixed a bug the clever version had been hiding: it matched only
`*rsa` and `*.pem`, silently skipping every ed25519 key.

## Where platform differences do belong

Not in the function body. Use `platforms:` on a catalog alias, or `Preamble()`
on a backend, so the difference is declared rather than branched on at runtime.

## Checking it

```sh
zsh -n file && bash -n file          # necessary, nowhere near sufficient
```

Both pass on every bug above. What actually catches them:

```sh
make golden && git diff testdata/golden    # bash and zsh output, both platforms
make check                                 # includes tests that ban these constructs
```

And then compare behaviour between the two dialects — see the `verify` skill.
