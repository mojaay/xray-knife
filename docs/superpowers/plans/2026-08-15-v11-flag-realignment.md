# v11 CLI Flag Realignment — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every shorthand flag mean exactly one thing across the whole `xray-knife` command tree, enforce it with a build-breaking test, and ship the change as v11 with a precise migration hint for every removed shorthand — plus three non-breaking UX fixes (`subs fetch` next-step hint, `subs fetch` parse-error reporting, root quickstart examples).

**Architecture:** A single canonical map (letter → the one long flag name it may bind to) lives in `cmd/shorthands.go`. `cmd/shorthands_test.go` walks the cobra tree from `rootCmd` and fails the build on any deviation. Because today's surface has 43 deviating bindings (30 distinct flags, several repeated across the four `proxy` subcommands), the test ships with a **ratchet list** of grandfathered violations; each subsequent task deletes its entries from that list, so every task leaves the build green and the final task leaves the list empty. Removed shorthands get a `SetFlagErrorFunc` on `rootCmd` that turns pflag's bare `unknown shorthand flag: 'k' in -k` into a migration instruction.

**Tech Stack:** Go 1.26, `github.com/spf13/cobra` v1.10.2, `github.com/spf13/pflag` v1.0.9 (currently an indirect dependency — Task 1 promotes it to direct). Tests use Go's standard `testing` package with no third-party assertion library, matching `cmd/subs/subscription_test.go`.

**Spec:** Design rationale is inline in this document (see "Design Rationale" below). There is no separate spec file.

## Global Constraints

- Go 1.26; module path becomes `github.com/lilendian0x00/xray-knife/v11` (Task 10).
- No new third-party dependencies. `pflag` moves from indirect to direct; nothing else is added.
- Tests: standard `testing` only. No `testify`, no assertion helpers.
- `rootCmd.Version` becomes `"11.0.0"` — set once in Task 10, not before.
- Long flag names that change keep the old name as a deprecated alias via `flags.MarkDeprecated`. **Shorthands never get aliases** — one FlagSet permits one binding per letter, so a moved letter is a hard break.
- No invocation may silently change meaning in v11. Any v10 command line either behaves identically, or errors with a migration hint. This is why `cfscanner -e` and `proxy -t` are left unbound (see Design Rationale §4).
- Every task ends with `go build ./... && go test ./...` green and a commit.

---

## Design Rationale

### §1 The canon already exists

`cmd/proxy/proxy.go:31-42` already registers exactly the right persistent set:

```
-a addr   -c config   -e insecure   -f file
-i stdin  -p port     -v verbose    -z core
```

`net tcp` (`-c` only) and `webui` (`-a`, `-p`) already comply. This plan promotes that set to a program-wide rule and fixes everyone else. It is not inventing a new convention.

### §2 Whitelist, not blocklist

Every shorthand in the tree must appear in `canonicalShorthands`. A new flag with an unregistered letter fails the build, forcing the cross-command decision at authoring time rather than at bug-report time. pflag panics on duplicate shorthands *within one FlagSet* but cannot see across commands — that blind spot is the root cause of all 30 current deviations.

### §3 Shorthands are earned

`cfscanner` binds 20 shorthands for 20 flags — 100% coverage — and every one of its collisions traces to that exhaustion. Meanwhile `proxy inbound` already ships 16 long-only flags (`--chain-*`, `--dns*`, `--health-*`, `--blacklist-*`) with no complaints. Bar for keeping a shorthand: *typed interactively, more than once a week*. Booleans that already default correctly almost never clear it.

### §4 Migration severity — why two letters stay unbound

| Group | Old → new | Failure mode | Handling |
|---|---|---|---|
| A | letter becomes unbound | `unknown shorthand flag` — loud | `SetFlagErrorFunc` rewrites it into a migration hint (Task 7) |
| B | letter rebound, **type changes** | parse error — loud | Rebind immediately. `cfscanner -c` gains a `://` guard |
| C | letter rebound, **type compatible** | **silent wrong behavior** | Leave unbound in v11; bind in v12 |

Group C is exactly two letters:

- **`cfscanner -e`** — v10 `--shuffle-subnet` (bool) → v11 `--insecure` (bool). A stale script would silently get weakened TLS instead of a shuffled scan.
- **`proxy -t`** — v10 `--rotate` (uint32) → v11 `--threads` (uint16). `-t 60` would silently change from "rotate every 60s" to "60 test threads".

Both stay **unbound** in v11 with a migration hint, and are bound to canon in v12. Cost: one release where `cfscanner --insecure` and `proxy --threads` are long-only. Benefit: v11 is provably free of silent behavior changes.

### §5 Contested-letter rulings

| Letter | Claimants | Winner | Reasoning |
|---|---|---|---|
| `-p` | port / speedtest / proxy-url | **port** | Universal convention (ssh, nc, curl). "p" for speedtest has no mnemonic. speedtest → `-S`; `--proxy` → long-only |
| `-a` | addr / amount / user-agent | **addr** | Two commands already use it, plus convention |
| `-u` | url / timeout / transport | **url** | Four commands vs one each |
| `-t` | threads / rotate | **threads** | Two vs one; `--rotate` → `-R` |
| `-r` | remark / retry / rip | **remark** | `--rip` is bool default-true (never typed); `--retry` rarely tuned |
| `-b` | body / batch | **body** | Two vs one; `--batch` defaults to 0=auto |
| `-j` | json / inbound-protocol | **json** | `--inbound` is set once per session |
| `-s` | sort / subnets | **subnets** | `--sort` is bool default-true; `--subnets` is cfscanner's primary input and is `MarkFlagRequired` |
| `-v` | verbose / version | **verbose** | Four subcommands read `-v` as verbose while root reads it as version. Root moves to `-V` |

### §6 Explicit non-goals

These were considered and **rejected** for this release:

- **`--save-db` stays `false`** in both `cmd/http/http.go` and `cmd/cfscanner/cfscanner.go`. Do not change it.
- **`--prescan` stays `false`** and is not auto-enabled by list size. Do not change it.
- **No `-q` / `--quiet` flag.** `q` is therefore *not* in the canonical map.
- **No `--no-color` flag.** `fatih/color` already honors the `NO_COLOR` environment variable and auto-disables on a non-TTY.
- `--dedup-semantic` defaulting to `true` already landed on master before this plan; no task covers it.

---

## File Structure

| Path | Status | Responsibility |
|---|---|---|
| `cmd/shorthands.go` | Create | `canonicalShorthands` map, `grandfathered` ratchet list, `removedShorthands` migration table, `flagErrorFunc`. Single source of truth for the CLI letter surface. |
| `cmd/shorthands_test.go` | Create | Walks the cobra tree from `rootCmd`; fails on unregistered shorthands, mismatched bindings, one-name-two-letters, and stale ratchet entries. |
| `cmd/root.go` | Modify | `-V` for `--version`, persistent `--db`, `Example` quickstart block, wire `flagErrorFunc`, version from build info. |
| `cmd/cfscanner/cfscanner.go:117-141` | Modify | 13 flag rebinds/demotions; `--output` → `--out`; `--config` `://` guard. |
| `cmd/http/http.go:605-665` | Modify | 5 flag rebinds/demotions; `--thread` → `--threads`. |
| `cmd/proxy/proxy.go:39` | Modify | `--port` `string` → `uint16`. |
| `cmd/proxy/shared.go:103-108,212` | Modify | `--rotate` → `-R`; `--batch`, `--concurrency` → `--threads` demotions; port conversion at the `pkg/proxy` boundary. |
| `cmd/proxy/inbound.go:27-30` | Modify | Demote `--inbound`, `--transport`, `--uuid`, `--inbound-config` to long-only. |
| `cmd/proxy/system.go:31-34` | Modify | Same four demotions. |
| `cmd/subs/fetch.go` | Modify | `--useragent` → `--user-agent` long-only; `--proxy` long-only; parse-error counting; next-step hint. |
| `cmd/subs/add.go:46`, `cmd/subs/update.go:76` | Modify | `--user-agent` long-only. |
| `cmd/subs/list_configs.go` | Modify | `--id` → `--sub-id`, add `-l` to `--limit`. |
| `utils/xkhome/xkhome.go` | Create | `Dir()` and `DBPath()` — resolves `--db` flag > `$XRAY_KNIFE_HOME` > `~/.xray-knife`. |
| `utils/xkhome/xkhome_test.go` | Create | Precedence tests. |
| `cmd/webui/webui.go:96`, `pkg/proxy/netns/state.go:30`, `pkg/proxy/sysproxy/sysproxy.go:31`, `pkg/proxy/sysproxy/sysproxy_linux.go:263`, `web/service_manager.go:27` | Modify | Replace hardcoded `~/.xray-knife` with `xkhome.Dir()`. |
| `build.sh`, `.github/workflows/build.yaml` | Modify | Inject version via `-ldflags -X`. |
| `go.mod` + every import | Modify | `/v10` → `/v11`; promote `pflag` to direct. |
| `MIGRATION-v11.md` | Create | Complete old→new table for users. |
| `README.md`, `docs/zh_README.md` | Modify | ~15 shorthand sites each; new quickstart. |

---

## Task 1: Shorthand registry and ratchet test

**Files:**
- Create: `cmd/shorthands.go`
- Create: `cmd/shorthands_test.go`
- Modify: `go.mod` (promote `pflag` to direct)

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `canonicalShorthands map[string]string` — letter → required long name.
  - `grandfathered map[string]bool` — keys are `"<command path> -<letter>"`, e.g. `"xray-knife cfscanner -c"`. Later tasks delete entries.
  - `visitShorthands(root *cobra.Command, fn func(path string, f *pflag.Flag))` — walks the tree, calling `fn` once per command-owned flag that declares a shorthand.

- [ ] **Step 1: Promote pflag to a direct dependency**

```bash
go get github.com/spf13/pflag@v1.0.9
go mod tidy
```

Expected: `go.mod` now lists `github.com/spf13/pflag v1.0.9` in the direct require block (no `// indirect` comment).

- [ ] **Step 2: Write the failing test**

Create `cmd/shorthands_test.go`:

```go
package cmd

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// TestShorthandsAreCanonical asserts every shorthand in the command tree binds
// to the one long name canonicalShorthands assigns it. Known violations are
// listed in grandfathered and are removed as each command is realigned.
func TestShorthandsAreCanonical(t *testing.T) {
	var violations []string

	visitShorthands(rootCmd, func(path string, f *pflag.Flag) {
		key := fmt.Sprintf("%s -%s", path, f.Shorthand)
		want, registered := canonicalShorthands[f.Shorthand]
		ok := registered && want == f.Name
		if ok {
			if grandfathered[key] {
				t.Errorf("%s now binds --%s correctly; delete it from grandfathered in cmd/shorthands.go", key, f.Name)
			}
			return
		}
		if grandfathered[key] {
			return
		}
		if !registered {
			violations = append(violations, fmt.Sprintf("%s binds --%s but -%s is not in canonicalShorthands", key, f.Name, f.Shorthand))
			return
		}
		violations = append(violations, fmt.Sprintf("%s binds --%s but -%s is reserved for --%s", key, f.Name, f.Shorthand, want))
	})

	if len(violations) > 0 {
		sort.Strings(violations)
		t.Errorf("shorthand violations:\n  %s", strings.Join(violations, "\n  "))
	}
}

// TestOneNameOneShorthand asserts a given long flag name never carries two
// different shorthands across the tree (catches --out/--output style drift).
func TestOneNameOneShorthand(t *testing.T) {
	seen := map[string]string{} // long name -> shorthand
	where := map[string]string{}

	visitShorthands(rootCmd, func(path string, f *pflag.Flag) {
		if prev, ok := seen[f.Name]; ok && prev != f.Shorthand {
			t.Errorf("--%s is -%s in %s but -%s in %s", f.Name, prev, where[f.Name], f.Shorthand, path)
			return
		}
		seen[f.Name] = f.Shorthand
		where[f.Name] = path
	})
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./cmd/ -run TestShorthands -v`
Expected: FAIL — `undefined: visitShorthands`, `undefined: canonicalShorthands`, `undefined: grandfathered`.

- [ ] **Step 4: Write the registry**

Create `cmd/shorthands.go`:

```go
package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// canonicalShorthands maps a shorthand letter to the ONE long flag name it may
// bind to, anywhere in the command tree. Adding an entry is a deliberate
// CLI-surface decision: cmd/shorthands_test.go fails the build on any
// deviation, so a new flag cannot quietly claim a letter another command
// already uses for something else.
//
// Letters absent from this map may not be used as shorthands at all. If a flag
// needs one, either it takes a free letter (added here) or it stays long-only.
// Long-only is the default for anything not typed interactively week to week.
var canonicalShorthands = map[string]string{
	"a": "addr",
	"b": "body",
	"c": "config",
	"d": "mdelay",
	"e": "insecure",
	"f": "file",
	"h": "help", // added by cobra
	"i": "stdin",
	"j": "json",
	"l": "limit",
	"m": "method",
	"o": "out",
	"p": "port",
	"r": "remark",
	"s": "subnets",
	"t": "threads",
	"u": "url",
	"v": "verbose",
	"w": "workers",
	"x": "type",
	"y": "yes",
	"z": "core",
	"R": "rotate",
	"S": "speedtest",
	"V": "version",
}

// grandfathered lists shorthand bindings that predate canonicalShorthands and
// have not been realigned yet. Keys are "<command path> -<letter>".
//
// This list only shrinks. Each realignment task deletes its own entries, and
// the test fails if an entry stops violating (so nothing goes stale). It is
// empty by the end of the v11 realignment; do not add to it.
var grandfathered = map[string]bool{
	"xray-knife -v": true,

	"xray-knife http -t": true,
	"xray-knife http -p": true,
	"xray-knife http -a": true,
	"xray-knife http -r": true,
	"xray-knife http -s": true,

	"xray-knife cfscanner -p": true,
	"xray-knife cfscanner -c": true,
	"xray-knife cfscanner -u": true,
	"xray-knife cfscanner -e": true,
	"xray-knife cfscanner -i": true,
	"xray-knife cfscanner -o": true,
	"xray-knife cfscanner -r": true,
	"xray-knife cfscanner -k": true,
	"xray-knife cfscanner -d": true,
	"xray-knife cfscanner -m": true,
	"xray-knife cfscanner -C": true,
	"xray-knife cfscanner -E": true,
	"xray-knife cfscanner -P": true,

	"xray-knife proxy inbound -t": true,
	"xray-knife proxy inbound -b": true,
	"xray-knife proxy inbound -n": true,
	"xray-knife proxy inbound -u": true,
	"xray-knife proxy inbound -j": true,
	"xray-knife proxy inbound -g": true,
	"xray-knife proxy inbound -I": true,
	"xray-knife proxy system -t":  true,
	"xray-knife proxy system -b":  true,
	"xray-knife proxy system -n":  true,
	"xray-knife proxy system -u":  true,
	"xray-knife proxy system -j":  true,
	"xray-knife proxy system -g":  true,
	"xray-knife proxy system -I":  true,
	"xray-knife proxy app -t":     true,
	"xray-knife proxy app -b":     true,
	"xray-knife proxy app -n":     true,
	"xray-knife proxy tun -t":     true,
	"xray-knife proxy tun -b":     true,
	"xray-knife proxy tun -n":     true,

	"xray-knife subs add -a":    true,
	"xray-knife subs update -a": true,
	"xray-knife subs fetch -a":  true,
	"xray-knife subs fetch -p":  true,
}

// visitShorthands walks the command tree and calls fn once for every flag that
// declares a shorthand, using the flag set the command itself owns (local +
// its own persistent flags). Inherited flags are skipped so a parent's
// persistent flag is reported once, against the parent.
func visitShorthands(root *cobra.Command, fn func(path string, f *pflag.Flag)) {
	root.InitDefaultHelpFlag()
	root.InitDefaultVersionFlag()

	inherited := map[string]bool{}
	root.InheritedFlags().VisitAll(func(f *pflag.Flag) { inherited[f.Name] = true })

	root.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Shorthand == "" || inherited[f.Name] {
			return
		}
		fn(root.CommandPath(), f)
	})

	for _, child := range root.Commands() {
		visitShorthands(child, fn)
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./cmd/ -run 'TestShorthands|TestOneName' -v`
Expected: PASS. If `TestShorthandsAreCanonical` reports a grandfathered entry that is not actually violating, delete that key. If it reports an unlisted violation, the enumeration above missed a flag — add the key and note it in the commit message.

- [ ] **Step 6: Verify the ratchet actually bites**

Temporarily add `flags.BoolVarP(&config.Verbose, "bogus", "Q", false, "temp")` to `cmd/http/http.go` in `addFlags`, then run `go test ./cmd/ -run TestShorthands`.
Expected: FAIL with `binds --bogus but -Q is not in canonicalShorthands`. Remove the temporary flag and re-run to confirm PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/shorthands.go cmd/shorthands_test.go go.mod go.sum
git commit -m "feat(cli): add canonical shorthand registry with ratchet test"
```

---

## Task 2: Realign cfscanner flags

**Files:**
- Modify: `cmd/cfscanner/cfscanner.go:117-141`
- Modify: `cmd/shorthands.go` (delete 13 `cfscanner` ratchet entries)
- Test: `cmd/shorthands_test.go` (existing), `cmd/cfscanner/cfscanner_test.go` (create)

**Interfaces:**
- Consumes: `canonicalShorthands`, `grandfathered` from Task 1.
- Produces: `validateConfigLink(link string) error` in `package cfscanner` — returns a migration-hint error when `--config` receives something with no `://`.

`cfscanner` goes from 20 shorthands to 9. Per Design Rationale §4, `-e` stays **unbound** in v11 (Group C) even though `--insecure` is its canonical home.

| Flag | v10 | v11 |
|---|---|---|
| `--config` | `-C` | `-c` |
| `--insecure` | `-E` | *(long-only until v12)* |
| `--port` | `-P` | `-p` |
| `--out` (was `--output`) | `-o` | `-o` |
| `--subnets` | `-s` | `-s` |
| `--threads` | `-t` | `-t` |
| `--body` | `-b` | `-b` |
| `--verbose` | `-v` | `-v` |
| `--speedtest` | `-p` | `-S` |
| `--speedtest-top` | `-c` | *(long-only)* |
| `--timeout` | `-u` | *(long-only)* |
| `--retry` | `-r` | *(long-only)* |
| `--only-speedtest` | `-k` | *(long-only)* |
| `--download-mb` | `-d` | *(long-only)* |
| `--upload-mb` | `-m` | *(long-only)* |
| `--shuffle-subnet` | `-e` | *(long-only)* |
| `--shuffle-ip` | `-i` | *(long-only)* |

- [ ] **Step 1: Write the failing test**

Create `cmd/cfscanner/cfscanner_test.go`:

```go
package cfscanner

import "testing"

func TestValidateConfigLink(t *testing.T) {
	cases := []struct {
		name    string
		link    string
		wantErr bool
	}{
		{"empty is fine", "", false},
		{"vless link", "vless://uuid@host:443?security=tls", false},
		{"v10 speedtest-top value", "10", true},
		{"bare host", "example.com", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConfigLink(tc.link)
			if tc.wantErr && err == nil {
				t.Fatalf("validateConfigLink(%q) = nil, want error", tc.link)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateConfigLink(%q) = %v, want nil", tc.link, err)
			}
		})
	}
}

func TestValidateConfigLinkMentionsSpeedtestTop(t *testing.T) {
	err := validateConfigLink("10")
	if err == nil {
		t.Fatal("expected an error for a bare integer")
	}
	if !contains(err.Error(), "--speedtest-top") {
		t.Fatalf("error %q should point users at --speedtest-top", err)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		len(haystack) > 0 && indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/cfscanner/ -run TestValidateConfigLink -v`
Expected: FAIL with `undefined: validateConfigLink`.

- [ ] **Step 3: Implement the guard**

Add to `cmd/cfscanner/cfscanner.go`:

```go
// validateConfigLink rejects a --config value that is not a proxy link. In v10
// the -c shorthand meant --speedtest-top, so a stale script passes an integer
// here; say so instead of failing later with an opaque parse error.
func validateConfigLink(link string) error {
	if link == "" || strings.Contains(link, "://") {
		return nil
	}
	return fmt.Errorf("--config expects a proxy link (e.g. vless://...), got %q\n"+
		"note: -c was --speedtest-top before v11; use --speedtest-top %s for the old behaviour", link, link)
}
```

Ensure `fmt` and `strings` are imported.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./cmd/cfscanner/ -run TestValidateConfigLink -v`
Expected: PASS.

- [ ] **Step 5: Rewrite the flag registrations**

Replace the body of `init()` in `cmd/cfscanner/cfscanner.go` with:

```go
func init() {
	f := CFscannerCmd.Flags()

	// Shorthands follow the canonical map in cmd/shorthands.go. Anything not
	// typed interactively week to week is long-only.
	f.StringSliceVarP(&cliConfig.Subnets, "subnets", "s", nil, "Subnet(s) or file containing subnets (e.g., \"1.1.1.1/24,2.2.2.2/16\")")
	f.IntVarP(&cliConfig.ThreadCount, "threads", "t", 100, "Count of threads for latency scan")
	f.StringVarP(&cliConfig.OutputFile, "out", "o", "results.csv", "Output file to save sorted results (in CSV format)")
	f.StringVarP(&cliConfig.ConfigLink, "config", "c", "", "Use a config link as a proxy to test IPs")
	f.IntVarP(&cliConfig.Port, "port", "p", 443, "TCP port to scan (Cloudflare also accepts 2053, 2083, 2087, 2096, 8443)")
	f.BoolVarP(&cliConfig.ShowTraceBody, "body", "b", false, "Show trace body output")
	f.BoolVarP(&cliConfig.Verbose, "verbose", "v", false, "Show verbose output with detailed errors")
	f.BoolVarP(&cliConfig.DoSpeedtest, "speedtest", "S", false, "Measure download/upload speed on the fastest IPs")

	// -e is deliberately left unbound in v11: it meant --shuffle-subnet in v10
	// and both flags are booleans, so rebinding it to --insecure would silently
	// change behaviour. It becomes --insecure in v12.
	f.BoolVar(&cliConfig.InsecureTLS, "insecure", false, "Allow insecure TLS connections for the proxy config")

	f.IntVar(&cliConfig.SpeedtestTop, "speedtest-top", 10, "Number of fastest IPs to select for speed testing")
	f.IntVar(&cliConfig.SpeedtestConcurrency, "speedtest-concurrency", 4, "Number of concurrent speed tests to run")
	f.IntVar(&cliConfig.SpeedtestTimeout, "speedtest-timeout", 30, "Total timeout in seconds for one IP's speed test")
	f.IntVar(&cliConfig.RequestTimeout, "timeout", 5000, "Individual request timeout (in ms)")
	f.IntVar(&cliConfig.RetryCount, "retry", 1, "Number of times to retry TCP connection on failure")
	f.BoolVar(&cliConfig.OnlySpeedtestResults, "only-speedtest", false, "Only display results that have successful speedtest data")
	f.IntVar(&cliConfig.DownloadMB, "download-mb", 20, "Custom amount of data to download for speedtest (in MB)")
	f.IntVar(&cliConfig.UploadMB, "upload-mb", 10, "Custom amount of data to upload for speedtest (in MB)")
	f.BoolVar(&cliConfig.ShuffleSubnets, "shuffle-subnet", false, "Shuffle list of Subnets")
	f.BoolVar(&cliConfig.ShuffleIPs, "shuffle-ip", false, "Shuffle list of IPs")
	f.BoolVar(&cliConfig.Resume, "resume", false, "Resume scan from previous results (file or DB)")
	f.BoolVar(&cliConfig.SaveToDB, "save-db", false, "Save scan results to the database")
	f.StringVar(&cliConfig.BindInterface, "bind", "", "Bind outbound dials to a specific OS interface (e.g. eth0). Linux: needs CAP_NET_RAW.")

	// --output was renamed to --out in v11 to match http and subs.
	f.StringVar(&cliConfig.OutputFile, "output", "results.csv", "Deprecated alias for --out")
	_ = f.MarkDeprecated("output", "use --out")

	_ = CFscannerCmd.MarkFlagRequired("subnets")
}
```

Note: `--output` is registered as a separate flag bound to the same variable so the deprecation warning fires; cobra prints the deprecation notice and both write `cliConfig.OutputFile`.

- [ ] **Step 6: Wire the guard into the command**

In `cmd/cfscanner/cfscanner.go`, add a `PreRunE` to `CFscannerCmd` (or extend the existing one):

```go
PreRunE: func(cmd *cobra.Command, args []string) error {
	return validateConfigLink(cliConfig.ConfigLink)
},
```

- [ ] **Step 7: Delete the cfscanner ratchet entries**

In `cmd/shorthands.go`, delete all 13 keys beginning `"xray-knife cfscanner "`.

- [ ] **Step 8: Run the full test suite**

Run: `go build ./... && go test ./cmd/... -v`
Expected: PASS, including `TestShorthandsAreCanonical` with no cfscanner violations.

- [ ] **Step 9: Verify the help output by hand**

Run: `go run . cfscanner --help`
Expected: 9 shorthands (`-b -c -o -p -s -S -t -v` plus `-h`); no `-C`, `-E`, `-P`, `-e`, `-i`, `-k`, `-d`, `-m`, `-r`, `-u`.

- [ ] **Step 10: Commit**

```bash
git add cmd/cfscanner/ cmd/shorthands.go
git commit -m "feat(cfscanner)!: realign shorthands to canonical map"
```

---

## Task 3: Realign http flags

**Files:**
- Modify: `cmd/http/http.go:605-665`
- Modify: `cmd/shorthands.go` (delete 5 `http` ratchet entries)

**Interfaces:**
- Consumes: `canonicalShorthands`, `grandfathered`.
- Produces: nothing new.

| Flag | v10 | v11 |
|---|---|---|
| `--threads` (was `--thread`) | `-t` | `-t` |
| `--speedtest` | `-p` | `-S` |
| `--amount` | `-a` | *(long-only)* |
| `--rip` | `-r` | *(long-only)* |
| `--sort` | `-s` | *(long-only)* |

- [ ] **Step 1: Apply the flag edits**

In `cmd/http/http.go`, `addFlags`:

```go
	// was: flags.Uint16VarP(&config.ThreadCount, "thread", "t", 50, ...)
	flags.Uint16VarP(&config.ThreadCount, "threads", "t", 50, "Number of threads")
	flags.Uint16Var(&config.ThreadCount, "thread", 50, "Deprecated alias for --threads")
	_ = flags.MarkDeprecated("thread", "use --threads")
```

```go
	// was: flags.BoolVarP(&config.Speedtest, "speedtest", "p", false, ...)
	flags.BoolVarP(&config.Speedtest, "speedtest", "S", false, "Speed test with speed.cloudflare.com")
	// was: flags.Uint64VarP(&config.SpeedtestAmount, "amount", "a", 10000, ...)
	flags.Uint64Var(&config.SpeedtestAmount, "amount", 10000, "Download and upload amount (KB)")
```

```go
	// was: flags.BoolVarP(&config.GetIPInfo, "rip", "r", true, ...)
	flags.BoolVar(&config.GetIPInfo, "rip", true, "Receive real IP (csv)")
```

```go
	// was: flags.BoolVarP(&config.SortedByRealDelay, "sort", "s", true, ...)
	flags.BoolVar(&config.SortedByRealDelay, "sort", true, "Sort config links by their delay (fast to slow) in file output")
```

Leave `--save-db` and `--prescan` at `false` — see Design Rationale §6.

- [ ] **Step 2: Delete the http ratchet entries**

In `cmd/shorthands.go`, delete the 5 keys beginning `"xray-knife http "`.

- [ ] **Step 3: Run the tests**

Run: `go build ./... && go test ./cmd/... ./pkg/http/...`
Expected: PASS.

- [ ] **Step 4: Verify the deprecated alias still works**

Run: `go run . http --thread 4 --help`
Expected: cobra prints `Flag --thread has been deprecated, use --threads` on stderr, then help.

- [ ] **Step 5: Commit**

```bash
git add cmd/http/http.go cmd/shorthands.go
git commit -m "feat(http)!: realign shorthands, rename --thread to --threads"
```

---

## Task 4: Realign proxy flags

**Files:**
- Modify: `cmd/proxy/proxy.go:39`
- Modify: `cmd/proxy/shared.go:28,103-108,212`
- Modify: `cmd/proxy/inbound.go:27-30`
- Modify: `cmd/proxy/system.go:31-34`
- Modify: `cmd/shorthands.go` (delete the `proxy` ratchet entries)
- Test: `cmd/proxy/proxy_test.go` (existing — extend)

**Interfaces:**
- Consumes: `canonicalShorthands`, `grandfathered`.
- Produces: `parentFlags.listenPort` changes type from `string` to `uint16`. `pkg/proxy.Config.ListenPort` stays a `string` — `shared.go` converts at the boundary.

| Flag | v10 | v11 |
|---|---|---|
| `--rotate` | `-t` | `-R` |
| `--threads` (was `--concurrency`) | `-n` | *(long-only until v12)* |
| `--batch` | `-b` | *(long-only)* |
| `--transport` | `-u` | *(long-only)* |
| `--inbound` | `-j` | *(long-only)* |
| `--uuid` | `-g` | *(long-only)* |
| `--inbound-config` | `-I` | *(long-only)* |
| `--port` | `-p` string | `-p` uint16 |

Per §4, `-t` is **not** rebound to `--threads` in v11: `-t 60` is numerically valid under both meanings, so the change would be silent. `-t` is unbound in v11 and becomes `--threads` in v12.

- [ ] **Step 1: Change the port type**

`cmd/proxy/shared.go:28`, in `parentFlags`:

```go
	listenPort    uint16
```

`cmd/proxy/proxy.go:39`:

```go
	flags.Uint16VarP(&pf.listenPort, "port", "p", 9999, "Listen port number for the proxy server")
```

`cmd/proxy/shared.go:212`, where `pkg/proxy.Config` is built:

```go
		ListenPort:  strconv.FormatUint(uint64(p.listenPort), 10),
```

Add `strconv` to the imports.

- [ ] **Step 2: Rebind the rotation flags**

`cmd/proxy/shared.go`, in `addRotationFlags`:

```go
	flags.Uint32VarP(&r.rotationInterval, "rotate", "R", 300, "How often to rotate outbounds (seconds)")
	flags.Uint16VarP(&r.maximumAllowedDelay, "mdelay", "d", 3000, "Maximum allowed delay (ms) for testing configs during rotation")
	flags.Uint16Var(&r.batchSize, "batch", 0, "Number of configs to test per rotation (0=auto)")

	// --concurrency renamed to --threads for consistency with http and
	// cfscanner. -t is NOT bound here: it meant --rotate in v10 and both take
	// a number, so rebinding it would silently change behaviour. It becomes
	// --threads in v12.
	flags.Uint16Var(&r.concurrency, "threads", 0, "Number of concurrent test threads (0=auto)")
	flags.Uint16Var(&r.concurrency, "concurrency", 0, "Deprecated alias for --threads")
	_ = flags.MarkDeprecated("concurrency", "use --threads")
```

- [ ] **Step 3: Demote the inbound flags**

In both `cmd/proxy/inbound.go:27-30` and `cmd/proxy/system.go:31-34`, replace the four `VarP` calls with `Var`:

```go
	flags.StringVar(&inboundCmdRot.in.inboundProtocol, "inbound", "socks", "Inbound protocol to use (vless, vmess, socks)")
	flags.StringVar(&inboundCmdRot.in.inboundTransport, "transport", "tcp", "Inbound transport to use (tcp, ws, grpc, xhttp)")
	flags.StringVar(&inboundCmdRot.in.inboundUUID, "uuid", "random", "Inbound custom UUID to use (default: random)")
	flags.StringVar(&inboundCmdRot.in.inboundConfigLink, "inbound-config", "", "Custom config link for the inbound proxy")
```

Use `systemCmdRot` in `system.go`.

- [ ] **Step 4: Update the help examples**

`cmd/proxy/inbound.go`, in the `Example` block, change:

```
  xray-knife proxy inbound -f configs.txt -t 60           # rotate every 60s from file
```

to:

```
  xray-knife proxy inbound -f configs.txt -R 60           # rotate every 60s from file
```

- [ ] **Step 5: Write a test for the port conversion**

Add to `cmd/proxy/proxy_test.go`:

```go
func TestListenPortReachesServiceAsString(t *testing.T) {
	saved := pf.listenPort
	t.Cleanup(func() { pf.listenPort = saved })

	pf.listenPort = 1080
	cfg := pf.serviceConfig() // whatever shared.go:212's enclosing constructor is called

	if cfg.ListenPort != "1080" {
		t.Fatalf("ListenPort = %q, want \"1080\"", cfg.ListenPort)
	}
}
```

If the code at `shared.go:212` is inline rather than in a named function, extract it into a `func (p *parentFlags) serviceConfig() proxy.Config` first, then write this test against it.

- [ ] **Step 6: Run the tests**

Run: `go build ./... && go test ./cmd/... ./pkg/proxy/...`
Expected: PASS.

- [ ] **Step 7: Delete the proxy ratchet entries**

In `cmd/shorthands.go`, delete every key beginning `"xray-knife proxy "`.

- [ ] **Step 8: Verify by hand**

Run: `go run . proxy inbound --help`
Expected: `-R` for `--rotate`; no `-t`, `-b`, `-n`, `-u`, `-j`, `-g`, `-I`; `--port uint16` in the global block.

- [ ] **Step 9: Commit**

```bash
git add cmd/proxy/ cmd/shorthands.go
git commit -m "feat(proxy)!: realign shorthands, rename --concurrency to --threads"
```

---

## Task 5: Realign subs flags

**Files:**
- Modify: `cmd/subs/fetch.go:80-88`
- Modify: `cmd/subs/add.go:46`
- Modify: `cmd/subs/update.go:76`
- Modify: `cmd/subs/list_configs.go`
- Modify: `cmd/shorthands.go` (delete the 4 `subs` ratchet entries)

**Interfaces:**
- Consumes: `canonicalShorthands`, `grandfathered`.
- Produces: nothing new.

| Command | Flag | v10 | v11 |
|---|---|---|---|
| `subs add` | `--user-agent` | `-a` | *(long-only)* |
| `subs update` | `--user-agent` | `-a` | *(long-only)* |
| `subs fetch` | `--user-agent` (was `--useragent`) | `-a` | *(long-only)* |
| `subs fetch` | `--proxy` | `-p` | *(long-only)* |
| `subs list-configs` | `--sub-id` (was `--id`) | — | — |
| `subs list-configs` | `--limit` | — | `-l` |

- [ ] **Step 1: Edit subs fetch flags**

`cmd/subs/fetch.go`, in `addFlags`:

```go
	flags.Int64Var(&fc.config.SubscriptionID, "id", 0, "The ID of the subscription from the DB")
	flags.StringVarP(&fc.config.SubscriptionURL, "url", "u", "", "A one-off subscription URL to fetch from")
	flags.StringVar(&fc.config.UserAgent, "user-agent", "", "Custom User-agent to be used (overrides DB value)")
	flags.StringVar(&fc.config.UserAgent, "useragent", "", "Deprecated alias for --user-agent")
	_ = flags.MarkDeprecated("useragent", "use --user-agent")
	flags.StringVarP(&fc.config.OutputFile, "out", "o", "configs.txt", "Output file for fetched configs (pass an empty string to disable)")
	flags.StringVar(&fc.config.Proxy, "proxy", "", "Proxy to use for fetching the subscription")
	flags.BoolVar(&fc.config.FetchAll, "all", false, "Fetch from all enabled subscriptions in the DB")
	flags.StringVarP(&fc.config.FileInput, "file", "f", "", "File containing subscription URLs (one per line)")
	flags.IntVarP(&fc.config.Workers, "workers", "w", 3, "Number of concurrent workers for --file and --all modes")
```

- [ ] **Step 2: Edit subs add and update**

`cmd/subs/add.go:46` and `cmd/subs/update.go:76` — change `StringVarP(..., "user-agent", "a", ...)` to `StringVar(..., "user-agent", ...)`, dropping the shorthand argument.

- [ ] **Step 3: Edit subs list-configs**

`cmd/subs/list_configs.go`:

```go
	flags.Int64Var(&listSubID, "sub-id", 0, "Filter by subscription ID")
	flags.Int64Var(&listSubID, "id", 0, "Deprecated alias for --sub-id")
	_ = flags.MarkDeprecated("id", "use --sub-id")
	flags.IntVarP(&listLimit, "limit", "l", 50, "Maximum number of configs to display")
```

Match the existing variable names and types in that file; if `--id` is currently an `int`, keep `int` for both.

- [ ] **Step 4: Delete the subs ratchet entries**

In `cmd/shorthands.go`, delete the 4 keys beginning `"xray-knife subs "`.

- [ ] **Step 5: Run the tests**

Run: `go build ./... && go test ./cmd/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/subs/ cmd/shorthands.go
git commit -m "feat(subs)!: realign shorthands, unify --user-agent and --sub-id"
```

---

## Task 6: Root command — `-V`, `--db`, and the quickstart

**Files:**
- Create: `utils/xkhome/xkhome.go`
- Create: `utils/xkhome/xkhome_test.go`
- Modify: `cmd/root.go`
- Modify: `cmd/webui/webui.go:96`, `pkg/proxy/netns/state.go:30`, `pkg/proxy/sysproxy/sysproxy.go:31`, `pkg/proxy/sysproxy/sysproxy_linux.go:263`, `web/service_manager.go:27`
- Modify: `cmd/shorthands.go` (delete `"xray-knife -v"` — the list is now empty)

**Interfaces:**
- Consumes: `canonicalShorthands`, `grandfathered`.
- Produces:
  - `xkhome.Dir() (string, error)` — `$XRAY_KNIFE_HOME` if set, else `~/.xray-knife`. Creates the directory if missing.
  - `xkhome.DBPath(override string) (string, error)` — `override` if non-empty, else `Dir()/xray-knife.db`.

- [ ] **Step 1: Write the failing test**

Create `utils/xkhome/xkhome_test.go`:

```go
package xkhome

import (
	"path/filepath"
	"testing"
)

func TestDirPrefersEnvVar(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XRAY_KNIFE_HOME", tmp)

	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error = %v", err)
	}
	if got != tmp {
		t.Fatalf("Dir() = %q, want %q", got, tmp)
	}
}

func TestDBPathOverrideWins(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XRAY_KNIFE_HOME", tmp)

	want := filepath.Join(tmp, "custom.db")
	got, err := DBPath(want)
	if err != nil {
		t.Fatalf("DBPath() error = %v", err)
	}
	if got != want {
		t.Fatalf("DBPath(%q) = %q, want %q", want, got, want)
	}
}

func TestDBPathDefaultsUnderHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XRAY_KNIFE_HOME", tmp)

	want := filepath.Join(tmp, "xray-knife.db")
	got, err := DBPath("")
	if err != nil {
		t.Fatalf("DBPath() error = %v", err)
	}
	if got != want {
		t.Fatalf("DBPath(\"\") = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./utils/xkhome/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement xkhome**

Create `utils/xkhome/xkhome.go`:

```go
// Package xkhome resolves where xray-knife keeps its state: the database,
// system-proxy backups, and netns bookkeeping. Every caller goes through here
// so XRAY_KNIFE_HOME relocates all of it at once.
package xkhome

import (
	"os"
	"path/filepath"
)

// Dir returns the xray-knife state directory, creating it if missing.
// XRAY_KNIFE_HOME wins if set; otherwise ~/.xray-knife.
func Dir() (string, error) {
	dir := os.Getenv("XRAY_KNIFE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".xray-knife")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// DBPath returns the SQLite database path. A non-empty override (the --db
// flag) is used verbatim; otherwise the database lives under Dir().
func DBPath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "xray-knife.db"), nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./utils/xkhome/ -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Rewrite root.go**

`cmd/root.go` — replace `rootCmd`, `initConfig` and `init`:

```go
// dbPathOverride backs the persistent --db flag. Empty means "use the default
// under XRAY_KNIFE_HOME (or ~/.xray-knife)".
var dbPathOverride string

var rootCmd = &cobra.Command{
	Use:     "xray-knife",
	Short:   "Swiss Army Knife for xray-core & sing-box",
	Version: "11.0.0",
	Example: `  # 1. Add a subscription and pull its configs into the local DB.
  #    Fetched links are also written to configs.txt.
  xray-knife subs add --url "https://example.com/sub" --remark "My VPN"
  xray-knife subs fetch --all

  # 2. Test the fetched configs; working ones land in valid.txt, fastest first.
  xray-knife http -f configs.txt

  # 3. Run a local SOCKS proxy on 127.0.0.1:9999 that rotates through them.
  xray-knife proxy inbound -f valid.txt`,
}

func initConfig() {
	dbPath, err := xkhome.DBPath(dbPathOverride)
	if err != nil {
		customlog.Printf(customlog.Failure, "Could not resolve the database path: %v\n", err)
		os.Exit(1)
	}
	if err := database.InitDB(dbPath); err != nil {
		customlog.Printf(customlog.Failure, "Failed to initialize database: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// -v is verbose in every subcommand; keep the root consistent by moving
	// --version to -V rather than letting the same letter mean two things.
	rootCmd.Flags().BoolP("version", "V", false, "version for xray-knife")
	rootCmd.SetVersionTemplate("{{.Name}} {{.Version}}\n")

	rootCmd.PersistentFlags().StringVar(&dbPathOverride, "db", "",
		"Path to the xray-knife SQLite database (default: $XRAY_KNIFE_HOME/xray-knife.db, else ~/.xray-knife/xray-knife.db)")

	addSubcommandPalettes()
}
```

Drop the now-unused `log`, `filepath` and `os.UserHomeDir` usages; add the `utils/xkhome` import.

- [ ] **Step 6: Replace the other hardcoded home paths**

In each of `cmd/webui/webui.go:96`, `pkg/proxy/netns/state.go:30`, `pkg/proxy/sysproxy/sysproxy.go:31`, `pkg/proxy/sysproxy/sysproxy_linux.go:263`, `web/service_manager.go:27`, replace the `filepath.Join(home, ".xray-knife")` construction with a call to `xkhome.Dir()`, propagating the error the same way the surrounding function already handles errors.

- [ ] **Step 7: Delete the last ratchet entry**

In `cmd/shorthands.go`, delete `"xray-knife -v": true`. The `grandfathered` map is now empty:

```go
// grandfathered is empty: the v11 realignment cleared every legacy binding.
// Do not add entries — fix the flag instead.
var grandfathered = map[string]bool{}
```

- [ ] **Step 8: Run everything**

Run: `go build ./... && go test ./...`
Expected: PASS, with `TestShorthandsAreCanonical` reporting no violations and no stale grandfathered keys.

- [ ] **Step 9: Verify by hand**

```bash
go run . --help          # quickstart Example block appears
go run . -V              # prints "xray-knife 11.0.0"
XRAY_KNIFE_HOME=/tmp/xk-test go run . subs show   # uses /tmp/xk-test/xray-knife.db
```

- [ ] **Step 10: Commit**

```bash
git add cmd/root.go cmd/shorthands.go utils/xkhome/ cmd/webui/ pkg/proxy/ web/
git commit -m "feat(cli)!: move --version to -V, add --db and XRAY_KNIFE_HOME, add quickstart"
```

---

## Task 7: Migration hints for removed shorthands

**Files:**
- Modify: `cmd/shorthands.go` (add `removedShorthands`, `flagErrorFunc`)
- Modify: `cmd/root.go` (wire `SetFlagErrorFunc`)
- Modify: `cmd/shorthands_test.go` (add hint tests)

**Interfaces:**
- Consumes: `rootCmd`.
- Produces: `flagErrorFunc(cmd *cobra.Command, err error) error` — rewrites pflag's `unknown shorthand flag` error into a migration instruction when the letter is one v11 removed.

- [ ] **Step 1: Write the failing test**

Add to `cmd/shorthands_test.go`:

```go
func TestFlagErrorFuncExplainsRemovedShorthand(t *testing.T) {
	err := flagErrorFunc(rootCmd, errors.New("unknown shorthand flag: 'k' in -k"))
	msg := err.Error()

	if !strings.Contains(msg, "--only-speedtest") {
		t.Errorf("hint for -k should name --only-speedtest, got: %s", msg)
	}
	if !strings.Contains(msg, "v11") {
		t.Errorf("hint should mention the v11 realignment, got: %s", msg)
	}
}

func TestFlagErrorFuncPassesThroughUnknownLetters(t *testing.T) {
	orig := errors.New("unknown shorthand flag: 'Q' in -Q")
	if got := flagErrorFunc(rootCmd, orig); got.Error() != orig.Error() {
		t.Errorf("unmapped letter should pass through unchanged, got: %s", got)
	}
}

func TestEveryRemovedShorthandNamesALongFlag(t *testing.T) {
	for letter, hint := range removedShorthands {
		if !strings.Contains(hint, "--") {
			t.Errorf("removedShorthands[%q] = %q: must name the replacement long flag", letter, hint)
		}
	}
}
```

Add `errors` and `strings` to the test imports.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/ -run TestFlagError -v`
Expected: FAIL — `undefined: flagErrorFunc`, `undefined: removedShorthands`.

- [ ] **Step 3: Implement the hint table and error func**

Add to `cmd/shorthands.go`:

```go
// removedShorthands maps a letter that v10 accepted but v11 does not to the
// instruction a user needs. pflag's own error ("unknown shorthand flag") says
// nothing about what the letter used to do, which is the whole question a
// user hitting it has.
var removedShorthands = map[string]string{
	"k": "-k was --only-speedtest; use --only-speedtest",
	"C": "-C was --config; use -c",
	"E": "-E was --insecure; use --insecure (it becomes -e in v12)",
	"P": "-P was --port; use -p",
	"g": "-g was --uuid; use --uuid",
	"I": "-I was --inbound-config; use --inbound-config",
	"n": "-n was --concurrency; use --threads",
	"i": "-i was --shuffle-ip on cfscanner; use --shuffle-ip",
	"e": "-e was --shuffle-subnet on cfscanner; use --shuffle-subnet",
	"a": "-a was --amount on http and --user-agent on subs; use the long flag",
	"r": "-r was --rip on http and --retry on cfscanner; use the long flag",
	"s": "-s was --sort on http; use --sort",
	"u": "-u was --timeout on cfscanner and --transport on proxy; use the long flag",
	"d": "-d was --download-mb on cfscanner; use --download-mb",
	"m": "-m was --upload-mb on cfscanner; use --upload-mb",
	"b": "-b was --batch on proxy; use --batch",
	"j": "-j was --inbound on proxy; use --inbound",
	"t": "-t was --rotate on proxy; use -R (it becomes --threads in v12)",
	"p": "-p was --speedtest on http and cfscanner; use -S",
}

var unknownShorthandRE = regexp.MustCompile(`unknown shorthand flag: '(.)' in`)

// flagErrorFunc turns pflag's bare "unknown shorthand flag" into a migration
// instruction for letters the v11 realignment moved. Anything else passes
// through untouched.
func flagErrorFunc(cmd *cobra.Command, err error) error {
	m := unknownShorthandRE.FindStringSubmatch(err.Error())
	if m == nil {
		return err
	}
	hint, ok := removedShorthands[m[1]]
	if !ok {
		return err
	}
	return fmt.Errorf("%w\n\nshorthand flags were realigned in v11: %s\nsee MIGRATION-v11.md", err, hint)
}
```

Add `fmt` and `regexp` to the imports.

Note: several letters (`-p`, `-a`, `-r`, `-s`, `-u`, `-e`, `-i`, `-b`, `-j`, `-t`) are still valid shorthands on *some* commands. The hint only fires where pflag actually rejected the letter, so a valid use is never intercepted.

- [ ] **Step 4: Wire it into root**

In `cmd/root.go`'s `init()`:

```go
	rootCmd.SetFlagErrorFunc(flagErrorFunc)
```

Cobra resolves `FlagErrorFunc` by walking up to the root, so this covers every subcommand.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./cmd/ -run TestFlagError -v`
Expected: PASS (3 tests).

- [ ] **Step 6: Verify end to end**

```bash
go run . cfscanner -s 1.1.1.1/24 -k
```

Expected stderr includes:

```
unknown shorthand flag: 'k' in -k

shorthand flags were realigned in v11: -k was --only-speedtest; use --only-speedtest
see MIGRATION-v11.md
```

- [ ] **Step 7: Commit**

```bash
git add cmd/shorthands.go cmd/shorthands_test.go cmd/root.go
git commit -m "feat(cli): explain removed shorthands instead of bare pflag errors"
```

---

## Task 8: `subs fetch` — report parse failures and print the next step

**Files:**
- Modify: `cmd/subs/fetch.go:346-426`
- Test: `cmd/subs/fetch_test.go` (create)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `parseLinks` changes signature from
  `func (fc *FetchCommand) parseLinks(rawLinks []string, subID sql.NullInt64) []database.SubscriptionConfig`
  to
  `func (fc *FetchCommand) parseLinks(rawLinks []string, subID sql.NullInt64) ([]database.SubscriptionConfig, int)`
  where the `int` is the count of links whose protocol could not be parsed. Three call sites update: `fetchAllSubscriptions` (`fetch.go:218`), `fetchFromFile` (`fetch.go:307`), `doFetch` (`fetch.go:352`).

Today `parseLinks` recovers a panic with the comment "Silently skip", and a `CreateProtocol`/`Parse` error is dropped on the floor. The config is still saved with an unknown protocol, so the user sees a healthy-looking success line and later wonders why `--protocol vless` filters return nothing.

- [ ] **Step 1: Write the failing test**

Create `cmd/subs/fetch_test.go`:

```go
package subs

import (
	"database/sql"
	"testing"

	"github.com/lilendian0x00/xray-knife/v10/pkg/core"
)

func newTestFetchCommand() *FetchCommand {
	return &FetchCommand{
		config: &FetchConfig{},
		core:   core.NewAutomaticCore(false, false),
	}
}

func TestParseLinksCountsUnparsable(t *testing.T) {
	fc := newTestFetchCommand()

	links := []string{
		"vless://11111111-1111-1111-1111-111111111111@example.com:443?security=tls&type=tcp#ok",
		"this-is-not-a-config-link",
		"",
		"also://nonsense-protocol",
	}

	configs, unparsable := fc.parseLinks(links, sql.NullInt64{})

	if len(configs) != 3 {
		t.Fatalf("len(configs) = %d, want 3 (blank line skipped)", len(configs))
	}
	if unparsable != 2 {
		t.Fatalf("unparsable = %d, want 2", unparsable)
	}
}

func TestParseLinksAllGoodReportsZero(t *testing.T) {
	fc := newTestFetchCommand()

	links := []string{
		"vless://11111111-1111-1111-1111-111111111111@example.com:443?security=tls&type=tcp#ok",
	}

	configs, unparsable := fc.parseLinks(links, sql.NullInt64{})

	if len(configs) != 1 {
		t.Fatalf("len(configs) = %d, want 1", len(configs))
	}
	if unparsable != 0 {
		t.Fatalf("unparsable = %d, want 0", unparsable)
	}
}
```

Note: the import path is `/v10` until Task 10 renames the module; update it there along with everything else.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/subs/ -run TestParseLinks -v`
Expected: FAIL — `assignment mismatch: 2 variables but fc.parseLinks returns 1 value`.

- [ ] **Step 3: Change parseLinks to count failures**

`cmd/subs/fetch.go`:

```go
// parseLinks accepts the subscriptionID to correctly populate the struct. It
// returns the parsed configs plus the number of links whose protocol could not
// be determined — those are still saved, but with an unknown protocol, so the
// caller reports the count rather than leaving the user to discover it via an
// empty --protocol filter later.
func (fc *FetchCommand) parseLinks(rawLinks []string, subID sql.NullInt64) ([]database.SubscriptionConfig, int) {
	var dbConfigs []database.SubscriptionConfig
	var unparsable int
	now := time.Now().UTC()

	for _, link := range rawLinks {
		trimmedLink := strings.TrimSpace(link)
		if trimmedLink == "" {
			continue
		}

		dbConf := database.SubscriptionConfig{
			SubscriptionID: subID,
			ConfigLink:     trimmedLink,
			LastSeenAt:     database.NullTime{Time: now, Valid: true},
		}

		// Parse protocol info with panic recovery — malformed links must not
		// crash the program, but they do get counted.
		parsed := func() (ok bool) {
			defer func() {
				if r := recover(); r != nil {
					ok = false
				}
			}()
			proto, err := fc.core.CreateProtocol(trimmedLink)
			if err != nil {
				return false
			}
			if err := proto.Parse(); err != nil {
				return false
			}
			g := proto.ConvertToGeneralConfig()
			dbConf.Protocol = sql.NullString{String: g.Protocol, Valid: g.Protocol != ""}
			dbConf.Remark = sql.NullString{String: g.Remark, Valid: g.Remark != ""}
			return g.Protocol != ""
		}()

		if !parsed {
			unparsable++
		}

		dbConfigs = append(dbConfigs, dbConf)
	}
	return dbConfigs, unparsable
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./cmd/subs/ -run TestParseLinks -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Update the three call sites to report the count**

`fetch.go:218` in `fetchAllSubscriptions`:

```go
			dbConfigs, unparsable := fc.parseLinks(rawLinks, subID)
			if unparsable > 0 {
				customlog.Printf(customlog.Warning, "Subscription %d (%s): %d link(s) could not be parsed; saved with unknown protocol.\n", sub.ID, remark, unparsable)
			}
```

`fetch.go:307` in `fetchFromFile`:

```go
			dbConfigs, unparsable := fc.parseLinks(rawLinks, subID)
			if unparsable > 0 {
				customlog.Printf(customlog.Warning, "%s: %d link(s) could not be parsed; saved with unknown protocol.\n", rawURL, unparsable)
			}
```

`fetch.go:352` in `doFetch`:

```go
	dbConfigs, unparsable := fc.parseLinks(rawLinks, subscriptionID)
	if unparsable > 0 {
		customlog.Printf(customlog.Warning, "%d link(s) could not be parsed; saved with unknown protocol.\n", unparsable)
	}
```

- [ ] **Step 6: Add the next-step hint**

Add to `cmd/subs/fetch.go`:

```go
// printNextStep tells the user what to run against what was just fetched.
// Fetching is never the goal on its own, and the file it wrote is the input to
// the next command.
func (fc *FetchCommand) printNextStep(saved int) {
	if saved == 0 {
		return
	}
	if fc.config.OutputFile != "" {
		customlog.Printf(customlog.Info, "Next: xray-knife http -f %s\n", fc.config.OutputFile)
		return
	}
	customlog.Printf(customlog.Info, "Next: xray-knife http --from-db\n")
}
```

Call it as the last statement before the successful `return nil` in all three of `fetchAllSubscriptions` (`fetch.go:256`), `fetchFromFile` (`fetch.go:342`) and `doFetch` (`fetch.go:376`), passing `len(allConfigs)` / `len(allConfigs)` / `len(dbConfigs)` respectively. In the two concurrent modes, call it only when `failed == 0`.

- [ ] **Step 7: Run the tests**

Run: `go build ./... && go test ./cmd/subs/ -v`
Expected: PASS.

- [ ] **Step 8: Verify by hand**

```bash
printf 'vless://bad\nnot-a-link\n' > /tmp/xk-subs.txt
go run . subs fetch --url "file:///tmp/xk-subs.txt" || true
```

Expected: a warning naming the unparsable count, then `Next: xray-knife http -f configs.txt`. (If `file://` URLs are unsupported by `Subscription.FetchAll`, use any reachable subscription URL instead — the assertion is on the two new lines, not the source.)

- [ ] **Step 9: Commit**

```bash
git add cmd/subs/fetch.go cmd/subs/fetch_test.go
git commit -m "feat(subs): report unparsable links and print the next command"
```

---

## Task 9: Version from build info

**Files:**
- Modify: `cmd/root.go`
- Modify: `build.sh`
- Modify: `.github/workflows/build.yaml`

**Interfaces:**
- Consumes: nothing.
- Produces: `var version = "dev"` in `package cmd`, overridable via `-ldflags`; `resolveVersion() string` falls back to `debug.ReadBuildInfo()`.

The hardcoded `Version: "10.1.1"` at `cmd/root.go:25` drifts every release and makes source builds lie about what they are.

- [ ] **Step 1: Write the failing test**

Add to `cmd/shorthands_test.go` (or create `cmd/version_test.go`):

```go
func TestResolveVersionPrefersLdflags(t *testing.T) {
	saved := version
	t.Cleanup(func() { version = saved })

	version = "11.2.3"
	if got := resolveVersion(); got != "11.2.3" {
		t.Fatalf("resolveVersion() = %q, want %q", got, "11.2.3")
	}
}

func TestResolveVersionNeverEmpty(t *testing.T) {
	saved := version
	t.Cleanup(func() { version = saved })

	version = ""
	if got := resolveVersion(); got == "" {
		t.Fatal("resolveVersion() returned an empty string")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/ -run TestResolveVersion -v`
Expected: FAIL — `undefined: version`, `undefined: resolveVersion`.

- [ ] **Step 3: Implement**

In `cmd/root.go`:

```go
// version is stamped at build time:
//   go build -ldflags "-X github.com/lilendian0x00/xray-knife/v11/cmd.version=11.0.0"
// Left as "dev" for plain `go build`, which then falls back to the module
// version recorded by `go install`.
var version = "dev"

func resolveVersion() string {
	if version != "" && version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	if version != "" {
		return version
	}
	return "dev"
}
```

Set `Version: resolveVersion()` on `rootCmd`. Because `rootCmd` is a package-level `var`, either build it in `init()` or assign `rootCmd.Version = resolveVersion()` as the first statement of `init()`. Add `runtime/debug` to the imports.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./cmd/ -run TestResolveVersion -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Stamp the version in build.sh**

In `build.sh`, add near the top:

```bash
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
LDFLAGS="-s -w -X github.com/lilendian0x00/xray-knife/v11/cmd.version=${VERSION#v}"
```

and pass `-ldflags "$LDFLAGS"` to every `go build` invocation in the script.

- [ ] **Step 6: Stamp it in CI**

In `.github/workflows/build.yaml`, export the same `VERSION` (from `github.ref_name` on tag builds) before the build step so released binaries report the tag.

- [ ] **Step 7: Verify**

```bash
go build -ldflags "-X github.com/lilendian0x00/xray-knife/v11/cmd.version=11.0.0-test" -o /tmp/xk . && /tmp/xk -V
```

Expected: `xray-knife 11.0.0-test`.

- [ ] **Step 8: Commit**

```bash
git add cmd/root.go cmd/version_test.go build.sh .github/workflows/build.yaml
git commit -m "build: stamp version via ldflags instead of hardcoding it"
```

---

## Task 10: Module path bump to /v11

**Files:**
- Modify: `go.mod`
- Modify: every `.go` file importing `github.com/lilendian0x00/xray-knife/v10/...`

**Interfaces:**
- Consumes: nothing.
- Produces: module path `github.com/lilendian0x00/xray-knife/v11`.

Precedent: commit `54e132a` ("module: bump path /v9 → /v10") did exactly this.

- [ ] **Step 1: Rewrite the module path**

```bash
go mod edit -module github.com/lilendian0x00/xray-knife/v11
grep -rl 'xray-knife/v10' --include='*.go' . | xargs sed -i '' 's|xray-knife/v10|xray-knife/v11|g'
```

On Linux use `sed -i` without the empty-string argument.

- [ ] **Step 2: Catch stragglers in non-Go files**

```bash
grep -rn 'xray-knife/v10' --include='*.sh' --include='*.yaml' --include='*.yml' --include='*.md' .
```

Update `build.sh`'s ldflags path (set in Task 9) and any docs that name the import path.

- [ ] **Step 3: Verify nothing references v10**

Run: `grep -rn 'xray-knife/v10' . --exclude-dir=.git`
Expected: no output.

- [ ] **Step 4: Build and test**

Run: `go mod tidy && go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "module: bump path /v10 -> /v11"
```

---

## Task 11: Migration guide and docs

**Files:**
- Create: `MIGRATION-v11.md`
- Modify: `README.md` (~15 shorthand sites)
- Modify: `docs/zh_README.md` (~15 shorthand sites)

**Interfaces:**
- Consumes: the final flag surface from Tasks 2-6.
- Produces: user-facing documentation. No code.

- [ ] **Step 1: Write MIGRATION-v11.md**

Create `MIGRATION-v11.md` with:

- A one-paragraph "why": shorthands meant different things per command; `cfscanner -e` silently toggled `--shuffle-subnet` when users expected `--insecure`.
- The canonical letter table (copy `canonicalShorthands` from `cmd/shorthands.go`).
- A per-command old→new table (copy the tables from Tasks 2-5 of this plan).
- A "deferred to v12" section naming `cfscanner -e` and `proxy -t` and explaining that they are unbound in v11 precisely so no v10 command line silently changes meaning.
- The renamed long flags: `--thread`→`--threads`, `--output`→`--out`, `--useragent`→`--user-agent`, `--concurrency`→`--threads`, `--id`→`--sub-id` (all keep deprecated aliases).
- The new `--db` flag and `XRAY_KNIFE_HOME`.
- `--version` moving from `-v` to `-V`.

- [ ] **Step 2: Update README examples**

Find every shorthand in the README and reconcile it against the new surface:

```bash
grep -nE '(^|[^-[:alnum:]])-[a-zA-Z][ =]' README.md
```

Replace the quickstart section with the same three-step flow now in `rootCmd.Example`:

```bash
xray-knife subs add --url "https://example.com/sub" --remark "My VPN"
xray-knife subs fetch --all
xray-knife http -f configs.txt
xray-knife proxy inbound -f valid.txt
```

- [ ] **Step 3: Update the Chinese README**

Apply the same replacements to `docs/zh_README.md` (`grep -nE '(^|[^-[:alnum:]])-[a-zA-Z][ =]' docs/zh_README.md`). Translate only the surrounding prose that changes; the commands are identical.

- [ ] **Step 4: Verify every documented command actually runs**

For each fenced `xray-knife ...` example in both READMEs, run it with `--help` appended (which parses flags without executing) and confirm no `unknown shorthand flag` error:

```bash
grep -ho 'xray-knife [a-z-]* [^`]*' README.md | while read -r line; do
  eval "go run . ${line#xray-knife } --help" >/dev/null 2>&1 || echo "BROKEN: $line"
done
```

Expected: no `BROKEN:` lines. Investigate any that appear.

- [ ] **Step 5: Commit**

```bash
git add MIGRATION-v11.md README.md docs/zh_README.md
git commit -m "docs: add v11 migration guide, update examples for new shorthands"
```

---

## Self-Review Notes

**Coverage check against the design rationale:**

| Requirement | Task |
|---|---|
| Canonical registry | 1 |
| Build-breaking enforcement | 1 |
| Ratchet keeps every task green | 1, cleared in 2-6 |
| cfscanner realignment (13) | 2 |
| http realignment (5) | 3 |
| proxy realignment (7) | 4 |
| subs realignment (4) | 5 |
| root `-v` → `-V` (1) | 6 |
| `--db` / `XRAY_KNIFE_HOME` | 6 |
| Root quickstart Example block | 6 |
| Migration hints for removed letters | 7 |
| `subs fetch` parse-error reporting | 8 |
| `subs fetch` next-step hint | 8 |
| Version from ldflags | 9 |
| Module path `/v11` | 10 |
| Migration guide + docs | 11 |
| `--save-db` stays false | Non-goal (§6) — no task, verified untouched in Task 3 Step 1 |
| `--prescan` stays false | Non-goal (§6) — no task, verified untouched in Task 3 Step 1 |
| No `-q`/`--quiet` | Non-goal (§6) — `q` absent from `canonicalShorthands` |

**Type consistency:** `parseLinks` returns `([]database.SubscriptionConfig, int)` in Task 8 and is called with two return values at all three sites. `parentFlags.listenPort` is `uint16` in Task 4 and converted with `strconv.FormatUint` at the single `pkg/proxy.Config` boundary; `pkg/proxy.Config.ListenPort` stays `string` everywhere else. `xkhome.Dir()` and `xkhome.DBPath(string)` both return `(string, error)` and are used that way in Task 6.

**Known ordering constraint:** Task 8's test file imports `.../v10/pkg/core`; Task 10's `sed` rewrites it along with everything else. If Task 10 runs before Task 8, use `/v11` in that import instead.
