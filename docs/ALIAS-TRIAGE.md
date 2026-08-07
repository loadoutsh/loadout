# Alias triage

Sorting all 188 aliases into what becomes product and what stays personal. Counts describe the repo as it stood at triage time; eight have since been dropped outright, leaving 180.

Verdicts other than the resolved ones below are still proposals open to correction.

## Categories

| Category   | Meaning                                                                         |
| ---------- | ------------------------------------------------------------------------------- |
| **SHIP**   | Curated default. On for everyone, subject to `requires`.                        |
| **CONFIG** | Generated from user config. The shape ships, the values do not.                 |
| **PACK**   | Opt-in bundle for one tool or ecosystem. Off by default, enabled in one action. |
| **DROP**   | Personal, dead, or a bad default to ship to strangers.                          |

`PACK` exists because ship-or-drop forces a bad call on the 9 Flutter aliases: useless to most people, valuable to Flutter developers. Packs are how an opinionated tool stays opinionated without being bloated.

## Counts

| Verdict | Count |
| ------- | ----: |
| SHIP    |   101 |
| CONFIG  |    30 |
| PACK    |    34 |
| DROP    |    23 |
| **Sum** |   188 |

The 23 DROP entries include eight already removed from the repo (see Resolved below); the other 15 are removed when the catalog is built.

## config.aliases (44, was 48)

**SHIP** — `zcon` `zcon:n` `zrc` `ezrc` `omz` `downloads` `..` `...` `....` `cl` `me` `h` `date:now` `today` `wget` `j` `sa` `sal` `cg` `vs` `v` `v.` `sshcon`

**SHIP, platform-conditional** — `ctl` and its five `ctl:*` variants (systemctl on Linux, launchctl on macOS), `app` `update` `debi` (apt/dpkg vs brew), `ports` (the `netstat -tulanp` flags are Linux-only; macOS needs `lsof -i -P`)

**CONFIG** — `micasa` `sucasa` `micaddy` `sucaddy` generate from `hosts[]`. `ut` `ut.` become an `editors[]` entry, since the portable VS Code path is machine-specific.

**DROP** — `.files` `aka` `aka:n` `vars` `vars:n`. All five open the dotfiles repo in an editor, which `loadout edit` and `loadout ui` replace outright.

## docker.aliases (1, was 2)

**SHIP** — `dk`

**DROPPED** — `dkc`. Compose v1 reached end of life in July 2023 and is absent from current Docker installs.

## frameworks.aliases (24)

All four groups become packs. None belongs in a default install.

- **PACK `gatsby`** — `g:dev` `g:build` `g:new`
- **PACK `jekyll`** — `jk` `jk:new` `jk:serve` `jk:build` `jk:page` `jk:post`
- **PACK `flutter`** — `fl` and its 8 `fl:*` variants
- **PACK `exercism`** — `xsm` `xsm:d` `xsm:s` `xsm:o` `xsm:v`, plus `xsm:cd` as CONFIG (the snap path is machine-specific)

Gatsby is worth reconsidering even as a pack; the project has been largely dormant since the Netlify acquisition.

## git.aliases (18)

**SHIP** — `gpush` `gpull` `rnm` `repo` `gist` `issue`

**CONFIG** — `dpush` `dpull` `gcod` `gcop`. These hardcode `development` and `main`; both branch names belong in config.

**PACK `git-flow`** — `finit` `ff` `fr` `fh`

**Flagged, still open** — `clone` `create` `fork` `view`

## misc.aliases (42, was 45)

**SHIP** — `untar` (without the `sudo`) `wch` `wis` `ht` `py` `dev` `fast` `nema` `rds` `lh` `standup` `db` `rw` `inf` `hr` `hr:l`

**SHIP, needs updating** — `mip` pipes to `xsel`, which is Linux-only and needs `pbcopy` on macOS.

**DROPPED** — `dl` `dlp` (youtube-dl, effectively unmaintained since 2021), `neo` (neofetch, archived 2024).

**PACK `cloudflared`** — `cf` `cft` `cftr`, with `dns` as CONFIG (the tunnel name `dev` is personal)

**PACK `ansible`** — `ans` `sans` `ansp` `sansp`

**CONFIG** — `vpn-up` `vpn-down` `vup` `vdn` (the wireguard interface `ut`), `ov` `ovl` `ovc` `ovd` and especially `ovi`, which hardcodes a path to one specific `.ovpn` file. `ddb` ties to the binaries feature.

**DROP** — `jquery` (fetches one pinned jQuery version to the clipboard), `q` (your own `qotm`), `m` (`mdb`, unclear and uninstalled), `llm` (hardcoded `~/AnythingLLMDesktop`), `tor`, `organize`, `tr` (Travis CI has been in terminal decline since 2021)

## npm.aliases (36)

**SHIP** — the full npm set (`n` `ni` `nu` `ns` `nr` `nb` `nd`), the full yarn set (11), the full bun set (8), `cna` `cnat` `oc`

**CONFIG** — `16` `18` `20` `22` `24` generate from a configured node version list rather than being fixed

**DROP** — `cpa` `ycpa`. Probot is niche enough that it belongs in a user's own config, not a shipped default.

## projects.aliases (3)

**CONFIG** — `a11` `a11-wb` `inerds` all generate from `workspaces[]`, using the `tmux_session` field. Nothing to ship.

## tmux.aliases (12)

**SHIP** — all twelve. This is the cleanest section in the repo: consistent, generic, no personal values. `tss` and `tsr` depend on the `tmux-session` script shipping as a builtin.

## Resolved

Eight aliases removed from the repo outright. Each either shadowed a command users already know or pointed at software that is no longer maintained.

| Alias      | Was                      | Why dropped                                                     |
| ---------- | ------------------------ | --------------------------------------------------------------- |
| `ls` `la`  | `colorls`, `colorls -al` | Shadowed `ls`; broke POSIX-output scripts and failed without it |
| `del`      | `rm -rf`                 | Two characters for recursive force-delete                       |
| `ping`     | `ping -c 5`              | Silently capped a command that otherwise runs until interrupted |
| `dl` `dlp` | `youtube-dl …`           | youtube-dl effectively unmaintained since 2021                  |
| `neo`      | `neofetch`               | Archived in 2024                                                |
| `dkc`      | `docker-compose`         | Compose v1 end-of-life July 2023, absent from current Docker    |

The commented-out `exa` variants went with `ls`/`la`; `exa` was archived in 2023 and superseded by `eza`. If the modern equivalents are wanted later — `eza`, `yt-dlp`, `fastfetch`, `docker compose` — they come back as new catalog entries rather than as revivals of these.

## Still open

**`clone` `create` `fork` `view`.** Four extremely generic verbs claimed as global aliases. `view` in particular shadows a real command on many systems (the read-only vi mode). Recommend renaming under the `gh` prefix — `ghc`, `ghn`, `ghf`, `ghv` — or dropping.

## Open

1. Whether packs are enabled individually or by detected ecosystem. Detection is friendlier — spotting a `pubspec.yaml` and offering the Flutter pack — but implicit behaviour in a tool that writes your shell config is worth being careful about.
2. Where a user's own aliases live once packs exist: flat in `aliases.custom`, or in user-defined packs of their own.
