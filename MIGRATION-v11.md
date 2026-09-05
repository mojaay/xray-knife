# Migrating to xray-knife v11

v11 makes every shorthand flag mean exactly one thing across the whole tool.

Before v11, a letter's meaning depended on which command you typed it after.
`-c` was the config link in `http`, `parse`, `proxy` and `net`, but
`--speedtest-top` in `cfscanner`. `-u` was a URL in four commands, a timeout in
`cfscanner`, and an inbound transport in `proxy`. Most of those mismatches
failed loudly, but two did not:

- **`cfscanner -e`** toggled `--shuffle-subnet` while muscle memory from every
  other command said `--insecure`. Both are booleans, so it parsed cleanly and
  silently did the wrong thing.
- **`cfscanner -i`** toggled `--shuffle-ip` where `-i` means `--stdin`
  everywhere else.

A test now walks the command tree on every build and fails if any shorthand
deviates from the canonical table, so this cannot drift back.

## Canonical shorthands

These letters have one meaning program-wide:

| Letter | Flag | Letter | Flag |
|---|---|---|---|
| `-a` | `--addr` | `-r` | `--remark` |
| `-b` | `--body` | `-s` | `--subnets` |
| `-c` | `--config` | `-t` | `--threads` |
| `-d` | `--mdelay` | `-u` | `--url` |
| `-e` | `--insecure` | `-v` | `--verbose` |
| `-f` | `--file` | `-w` | `--workers` |
| `-i` | `--stdin` | `-x` | `--type` |
| `-j` | `--json` | `-y` | `--yes` |
| `-l` | `--limit` | `-z` | `--core` |
| `-m` | `--method` | `-R` | `--rotate` |
| `-o` | `--out` | `-S` | `--speedtest` |
| `-p` | `--port` | `-V` | `--version` |

Any flag not listed is long-only. That is deliberate: the alphabet only ran
short because rarely-typed flags were holding letters.

## What changed, per command

Where a shorthand was removed, the long flag still works and is unchanged.

### `cfscanner`

| Flag | v10 | v11 |
|---|---|---|
| `--config` | `-C` | `-c` |
| `--port` | `-P` | `-p` |
| `--out` (renamed from `--output`) | `-o` | `-o` |
| `--speedtest` | `-p` | `-S` |
| `--insecure` | `-E` | long-only *(see below)* |
| `--speedtest-top` | `-c` | long-only |
| `--timeout` | `-u` | long-only |
| `--retry` | `-r` | long-only |
| `--only-speedtest` | `-k` | long-only |
| `--download-mb` | `-d` | long-only |
| `--upload-mb` | `-m` | long-only |
| `--shuffle-subnet` | `-e` | long-only |
| `--shuffle-ip` | `-i` | long-only |

`-s`, `-t`, `-b`, `-v` are unchanged.

Passing a non-link to `--config` (what a v10 script does when `-c` carried a
number) now errors with a pointer to `--speedtest-top` instead of failing
later with an opaque message.

### `http`

| Flag | v10 | v11 |
|---|---|---|
| `--threads` (renamed from `--thread`) | `-t` | `-t` |
| `--speedtest` | `-p` | `-S` |
| `--amount` | `-a` | long-only |
| `--rip` | `-r` | long-only |
| `--sort` | `-s` | long-only |

### `proxy`

| Flag | v10 | v11 |
|---|---|---|
| `--rotate` | `-t` | `-R` |
| `--threads` (renamed from `--concurrency`) | `-n` | long-only *(see below)* |
| `--batch` | `-b` | long-only |
| `--transport` | `-u` | long-only |
| `--inbound` | `-j` | long-only |
| `--uuid` | `-g` | long-only |
| `--inbound-config` | `-I` | long-only |

`--port` is now parsed as a number, so a bad value fails immediately with a
clear message instead of reaching the proxy service as a string.

### `subs`

| Flag | v10 | v11 |
|---|---|---|
| `--user-agent` (`fetch`, renamed from `--useragent`) | `-a` | long-only |
| `--user-agent` (`add`, `update`) | `-a` | long-only |
| `--proxy` (`fetch`) | `-p` | long-only |
| `--sub-id` (`list-configs`, renamed from `--id`) | — | — |
| `--limit` (`list-configs`) | — | `-l` |

### Root

`--version` moved from `-v` to `-V`. Every subcommand reads `-v` as
`--verbose`; the root now agrees.

## Deferred to v12

Two letters are left **unbound** in v11 rather than rebound, so that no v10
command line silently changes meaning:

- **`cfscanner -e`** — was `--shuffle-subnet`, will become `--insecure`. Both
  are booleans, so rebinding now would silently weaken TLS for anyone with a
  stale script.
- **`proxy -t`** — was `--rotate`, will become `--threads`. Both take a number,
  so `-t 60` would silently change from "rotate every 60s" to "60 threads".

Until v12, use `--insecure` and `--threads` in full.

## Renamed long flags

All of these keep the old name as a deprecated alias, so nothing breaks
immediately:

| Old | New |
|---|---|
| `http --thread` | `http --threads` |
| `cfscanner --output` | `cfscanner --out` |
| `subs fetch --useragent` | `subs fetch --user-agent` |
| `proxy --concurrency` | `proxy --threads` |
| `subs list-configs --id` | `subs list-configs --sub-id` |

## New

- **`--db <path>`** — a persistent flag on every command, pointing at a
  specific SQLite database.
- **`XRAY_KNIFE_HOME`** — relocates the whole state directory (database,
  system-proxy backups, netns bookkeeping, Web UI credentials). Defaults to
  `~/.xray-knife`.
- **`xray-knife --help`** now opens with a three-step quickstart.
- **`xray-knife -V`** reports the version the binary was actually built from,
  stamped at build time rather than hardcoded.

## Fixed

**`go install` failed to link on Go 1.27** with
`relocation target golang.org/x/net/http2.(*Transport).connPool not defined`.
Go 1.27 switches `golang.org/x/net/http2` to a thin wrapper over the standard
library, which no longer has the unexported method that sing-box 1.13 reached
through `go:linkname`. sing-box is now 1.14.0, which dropped that hack. The
sing-box core was adapted to the 1.14 API along the way: WireGuard is an
endpoint rather than an outbound, and DNS transports are registered
explicitly. ([#64](https://github.com/lilendian0x00/xray-knife/issues/64))

**`http --speedtest` reported 0 Mbps on slow links.** The speed test moved
`--amount` (10 MB by default) per direction and treated the 30 s
`--speedtest-timeout` as pass/fail, so any link under ~2.7 Mbps failed both
directions and wrote `0` to the CSV. The timeout is now a measurement window:
whatever moved inside it is measured, and only a transfer that moved nothing is
reported as a failure. Single-config mode also prints the failure reason
instead of a silent 0. ([#69](https://github.com/lilendian0x00/xray-knife/issues/69))

**All hysteria2 configs failed `http` with a bare `EOF`** since the sing-box
1.13 stack, even when the tunnel itself worked. sing-quic wraps every
stream-read error in its own type, including `io.EOF`, and the standard
library compares against `io.EOF` by identity — so a clean end-of-stream
looked like a transport failure and the response was thrown away. The dialed
connection is now unwrapped, which also covers TUIC. ([#66](https://github.com/lilendian0x00/xray-knife/issues/66))

## If you hit an unknown shorthand

The error now tells you what the letter used to do:

```
$ xray-knife cfscanner -s 1.1.1.1/24 -k
unknown shorthand flag: 'k' in -k

shorthand flags were realigned in v11: -k was --only-speedtest; use --only-speedtest
see MIGRATION-v11.md
```

## Go module path

The import path is now `github.com/lilendian0x00/xray-knife/v11`.

```bash
go install github.com/lilendian0x00/xray-knife/v11@latest
```
