# Local Daemon

The local daemon is an optional acceleration layer. A normal `bu1ld build`
still works without it; when a daemon is running, supported project commands
are proxied to the daemon process. If the daemon cannot be reached, the CLI
falls back to the in-process runtime.

## Commands

```bash
go run ./cmd/bu1ld daemon status
go run ./cmd/bu1ld daemon start
go run ./cmd/bu1ld daemon stop
```

`cmd/daemon` exposes the same control surface for release packaging:

```bash
go run ./cmd/daemon status
go run ./cmd/daemon start
go run ./cmd/daemon stop
```

Use `--no-daemon` on the main CLI to force local execution:

```bash
go run ./cmd/bu1ld --no-daemon build
```

## State

The daemon is scoped to a workspace root. It writes state under `.bu1ld`:

- `.bu1ld/daemon.json` stores the local endpoint, pid, workspace, and start
  time.
- `.bu1ld/daemon.log` receives stdout and stderr from a background daemon
  started through `daemon start`.

Status checks read the state file and then verify the local HTTP endpoint. If
the endpoint is gone, the daemon is treated as stopped with stale state.

## Command Proxying

The first daemon iteration proxies graph-oriented project commands:

- `build`
- `test`
- `doctor`
- `graph`
- `tasks`
- `packages`
- `packages graph`
- `affected`

Mutating plugin registry commands and server lifecycle commands still execute
directly in the CLI process. This keeps the daemon useful as a runtime
foundation without making every command depend on it.

## Next Steps

The daemon currently creates a fresh command runtime for each proxied command.
That keeps behavior equivalent to local execution while establishing the
control channel. Future iterations can keep plugin processes, project graphs,
configuration caches, and file snapshots warm inside the daemon.
