---
name: verify
description: How to actually check a change to loadout works, beyond compiling. Use before claiming any change is done, when a test passes but something still seems wrong, or when writing a new test for generated shell, the web UI, downloads, or credentials.
---

# Verifying a change

Nearly every real bug in this project got past the tests. Not because the tests
were absent, but because they checked the wrong thing.

## The pattern

| What was checked        | What was broken                                                   |
| ----------------------- | ----------------------------------------------------------------- |
| Function parses         | An array key was rewritten; it parsed, defined, looked up nothing |
| `apply` renders         | `loadout env` wrote to stderr, so `eval` got nothing              |
| Asset served with token | A `<script>` tag cannot send a token; the page had no CSS or JS   |
| A guard exists          | It ran inside `$( )`, so its `exit` ended only the subshell       |
| Formula generated       | A missing platform produced `sha256 ""` and exit 0                |

Every one passes `go test`. Every one is broken for a user.

## Rules

**Make a new check fail before trusting it.** Break the thing deliberately,
confirm the check catches it, then fix it. A guard that has never failed has not
been tested — it has been written.

**Never assert only that something parses.** `zsh -n` and `bash -n` pass on all
three of the zsh-only constructs that bash gets wrong. Compare behaviour.

**Exercise the path a user takes, not a convenient approximation.** Passing a
token by hand with `curl` is not how a browser loads a stylesheet. Running the
binary from `/tmp` is not how someone runs it from `PATH`.

**Read the output, do not just check the exit code.** The tamper test on
`install.sh` "passed" once because a stray `cd` meant the script was never
found. `nothing installed` was true and meaningless.

## What to run

```sh
make check     # build, vet, test, format
make golden    # then READ git diff testdata/golden
```

The golden diff is what lands in every user's shell. Reviewing it is the point,
not re-recording it.

## By area

**Generated shell** — compare behaviour across dialects, not just output:

```sh
make install
loadout apply --shell zsh --platform linux --stdout > /tmp/a.zsh
loadout apply --shell bash --platform linux --stdout > /tmp/a.bash
diff <(grep '^alias ' /tmp/a.zsh | sort) <(grep '^alias ' /tmp/a.bash | sort)

# then run the functions with externals stubbed and compare stdout
for sh in zsh bash; do
  $sh -c 'code() { echo "code $*"; }
          source /tmp/a.'$sh'
          pw a11; pw nope; p missing/path'
done
```

**The web UI** — fetch assets the way a browser does, with no token and no
query string, and confirm the API still refuses one:

```sh
loadout ui --port 8800 --no-open --print-url
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8800/app.js       # 200
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8800/api/state    # 401
curl -s -o /dev/null -w '%{http_code}\n' -H 'Host: evil.com:8800' \
     -H "X-Loadout-Token: $TOK" http://127.0.0.1:8800/api/state             # 403
```

**Downloads and self-update** — the negative cases matter more. Serve a
tampered artefact and confirm nothing is installed and the existing binary is
untouched. `httptest.NewTLSServer` is required: plain HTTP is refused by design,
which will otherwise look like a test failure.

**Credentials** — there is only one keychain per user, so tests cannot sandbox
it. Use `credentials.NewFake()` for unit tests. Anything touching the real
keyring goes behind `LOADOUT_KEYRING_TEST=1` and cleans up after itself.

**Anything touching a user's real setup** — back it up first, keep credentials
with `--keep-credentials`, and verify the restore before deleting the backup.

## Before saying it is done

- `make check` passes
- `git diff testdata/golden` reviewed, not just regenerated
- the change was run through the real command, not only its unit test
- any new guard was watched failing
- `CHANGELOG.md` updated under `[Unreleased]` if users would notice
