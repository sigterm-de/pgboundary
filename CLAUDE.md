# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

A Go CLI wrapper around the `boundary` and `pgbouncer` external binaries. It lets IDE/database
tooling connect through a stable local PgBouncer proxy while HashiCorp Boundary (with Vault
dynamic credentials via OIDC) supplies rotating `username`/`password`/`port` behind the scenes.

## Build, test, lint

```bash
make build       # go build with version/commit/date ldflags injected into cmd.version etc.
make test        # go test -v ./...
make test-race   # go test -v -race ./...
make clean       # tidy, fmt, vet, golangci-lint (if installed), remove binary/dist
make release     # goreleaser release --snapshot --clean --skip=sign
make all         # clean -> build -> test
```

Run a single test:
```bash
go test -v ./config/... -run TestParseTarget
go test -v ./config/... -run TestLoadConfig/valid_config   # subtest by name
```

CI (`.github/workflows/build.yml`) runs on PRs to `main` with `go build -v ./...` and
`go test -v -race ./...` on Go 1.25. Releases (`release.yml`) trigger on `v*` tags via
GoReleaser, signing artifacts with cosign and generating SBOMs with Syft.

## Architecture

The codebase models the two-step connection lifecycle described in the README:

1. **`config`** (`config/config.go`) — parses `pgboundary.ini` (via `gopkg.in/ini.v1`) into a
   `Config` struct (`PgBouncer`, `Scopes`, `Auth`, `Targets`). It also hand-parses the referenced
   PgBouncer `conffile` (line-by-line, not via the ini library) to extract `pidfile`/`auth_file`
   paths, and resolves `workdir` relative to the location of `pgboundary.ini` itself. `Target`
   entries are space-separated `key=value` strings (e.g. `host=... target=... database=...`);
   if `database` is omitted it's derived from the target name by stripping a trailing `-ro`/`-rw`.
2. **`internal/boundary`** (`boundary.go`) — talks to the Boundary HTTP API (via
   `hashicorp/boundary/api`) to resolve scope/auth-method IDs, then shells out to the `boundary`
   CLI (`boundary authenticate`, `boundary connect`) as subprocesses because the connect
   websocket/local-proxy behavior isn't exposed by the API client. Connection credentials are
   read back from a JSON file the `boundary connect` subprocess writes to a temp dir.
3. **`internal/pgbouncer`** (`pgbouncer.go`) — writes each active connection as its own temp INI
   file (tagged with a `; boundary_pid=<pid>` comment for later lookup) and appends it to the main
   PgBouncer config via `%include`. Reloads PgBouncer with `SIGHUP` (or starts it via
   `pgbouncer --daemon` if not running); full shutdown uses `SIGTERM` plus config cleanup.
4. **`internal/process`** — thin `gopsutil`-based helpers (`IsProcessType`, `KillProcess`,
   `Processes`) shared by the boundary/pgbouncer packages to identify and signal OS processes by
   PID, and a package-level `Verbose` flag toggled by `-v`/`--verbose`.
5. **`cmd`** (Cobra) — `connect`, `list`, `shutdown`, `version`. `root.go`'s
   `PersistentPreRunE` loads `Cfg` (package-level `*config.Config` in `cmd`) once per invocation,
   either from `-c/--config` or by probing `./pgboundary.ini`, `~/.pgboundary/pgboundary.ini`,
   then `$XDG_CONFIG_HOME/pgboundary/pgboundary.ini` in order.

Key invariant: a PgBouncer include file's boundary PID comment is the only link between a named
connection and its underlying `boundary connect` process — `ShutdownConnection` and `list`
both parse it back out of the include file rather than tracking state elsewhere. When the last
boundary-backed connection is removed, PgBouncer itself is shut down and the config is cleaned.
Connection config files live under `<workdir>/.pgboundary-connections/<target>.ini` (not the OS
temp dir), one fixed path per target. Since cleanup normally only happens through that graceful
path, `pgbouncer.Reconcile` (`internal/pgbouncer/reconcile.go`) runs before every command (via
`root.go`'s `PersistentPreRunE`) to drop any `%include` entry left behind by a non-graceful exit
(crash, reboot, kill -9) — checked by whether the entry's file is missing or its `boundary_pid`
process is dead. It's also exposed directly as `pgboundary cleanup`.

## Conventions

- Errors are wrapped with `fmt.Errorf("...: %w", err)` and a short present-tense description of
  the failing action (e.g. `"failed to parse target %s: %w"`).
- External processes (`boundary`, `pgbouncer`) are invoked via `os/exec`, never re-implemented;
  don't add API-based replacements unless the underlying CLI truly can't do it.
- Verbose/debug logging goes through the package-level `process.Verbose` bool, checked with
  `if process.Verbose { ... }` — there's no structured logger.
- `cmd/*.go` commands each set `process.Verbose` in a `PreRun` (in addition to root's
  `PersistentPreRunE`); follow this pattern when adding a new command.
- Version metadata (`version`, `commit`, `buildDate` in `cmd/version.go`) is injected at build
  time via `-ldflags`; don't hardcode or read these another way.

## Configuration files (for local testing/manual verification)

`pgboundary.ini`, `pg_config.ini`, and `pg_auth` in the repo root are sample/dev config files
following the format described in README.md. The binary resolves its config in order: `-c` flag,
`./pgboundary.ini`, `~/.pgboundary/pgboundary.ini`, `$XDG_CONFIG_HOME/pgboundary/pgboundary.ini`.
