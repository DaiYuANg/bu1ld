# Architecture

bu1ld is a cross-language build tool built around a small DSL, a task graph,
an action cache, and an external plugin protocol. The CLI remains the main user
entry point, while `cmd/server`, `cmd/daemon`, and `cmd/lsp` host the
coordinator, future daemon runtime, and editor language server.

## Runtime

Every subcommand starts a fresh `arcgolabs/dix` application. The runtime wires:

- `config.Config`: project directory, config files, dotenv values, CLI flags,
  remote cache settings, and plugin registry settings.
- `dsl.Loader`: root build file loading, imports, package discovery, plugin
  declarations, and configuration cache.
- `plugin.Registry`: builtin plugins plus local/global/path-based process
  plugins.
- `engine.Engine`: task execution, event emission, local and remote cache use.
- `cache.Store`: local records, output blobs, config cache, and coordinator
  storage.
- `eventx.BusRuntime`: console task progress events.

This keeps command handlers thin: they load the project, ask the graph planner
for tasks, and hand execution to the engine.

## Configuration

Configuration is loaded with `configx` from defaults, optional config files,
dotenv files, and `BU1LD_` environment variables. Supported config files are
`bu1ld.yaml`, `bu1ld.toml`, `bu1ld.json`, and their `.bu1ld.*` variants.

The `env` field selects environment-specific dotenv files. With `env = "lan"`,
bu1ld loads `.env.lan.local`, `.env.lan`, `.env.local`, and `.env` from the
project directory.

Important configuration areas:

- `build_file`: root DSL file, default `build.bu1ld`.
- `cache_dir`: local cache directory, default `.bu1ld/cache`.
- `remote_cache.*`: remote action cache pull/push settings.
- `server.coordinator.listen_addr`: coordinator listen address.
- `plugin_registry.source`: external plugin registry source.

## DSL Loading

The root build file describes workspace metadata, plugins, toolchains, imports,
packages, and tasks. Imported files can use globs such as `tasks/**/*.bu1ld`.

The loader compiles DSL forms into a `build.Project`:

- `workspace` sets the workspace name and default task.
- `plugin` declares a plugin source and version.
- Plugin rule invocations expand into build tasks.
- `task` declares custom tasks with dependencies, inputs, outputs, commands,
  or actions.
- Workspace packages expose package-scoped task IDs like `apps/api:build`.

The configuration cache stores the evaluated project under
`.bu1ld/cache/config/project.bin`. It is invalidated by changes to the root
build file, imported files, import glob expansion, environment variables read
through `env(...)`, or external plugin binaries.

## Task Graph

The graph planner resolves requested task IDs, expands dependencies, detects
cycles, and returns execution order. Package dependencies can add same-name task
dependencies between workspace packages, which lets `--all` style commands
operate across monorepos without manually wiring every edge.

Task identity is string-based. Plain workspace tasks use their declared name;
package tasks use `<package>:<task>`.

## Execution

The engine executes planned tasks through one of three paths:

- `command`: direct argv execution for simple tasks.
- Builtin action handlers: native Go implementations for `docker`, `archive`,
  and `git`.
- `plugin.exec`: delegated execution back into an external process plugin.

Tasks declare inputs and outputs. Input fingerprints and action parameters form
the action key. Outputs are captured into content-addressed blobs so the local
and remote cache can restore them later.

Console output is event-driven. The engine publishes task started, cache hit,
noop, completed, and failed events; the app subscribes and formats them for the
CLI.

## Cache Boundaries

The local action cache and remote cache share the same model:

- Action record: task metadata, input hash state, output references.
- Blob: content-addressed output bytes.

The remote coordinator exposes the same records and blobs over HTTP. Go build
cache entries are stored beside bu1ld action records, but they keep Go's
`ActionID` and `OutputID` shape so the `GOCACHEPROG` adapter can interoperate
with the Go toolchain.

See [Remote Cache](remote-cache.md) for the coordinator API and LAN setup.
