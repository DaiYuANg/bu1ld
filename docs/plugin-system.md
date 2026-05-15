# Plugin System

Plugins let bu1ld add task rules without hardcoding every language or tool into
the core binary. A plugin can be a builtin Go implementation linked into bu1ld,
an external process written in any language, or a containerized process. External
plugins implement the JSON-RPC protocol over stdin/stdout.

## Plugin Boundary

Plugins are adapters from existing tools and ecosystems into the bu1ld runtime.
They should import existing project metadata when it exists, register tasks,
translate bu1ld fields into tool-specific options, declare inputs and outputs,
and connect tool execution to bu1ld cache and artifact handling.

Plugins should not duplicate mature ecosystem behavior unless there is a clear
runtime reason to do so. Prefer official toolchains and well-maintained
libraries for language-specific complexity:

- Go plugins should call the Go toolchain rather than reimplement package
  loading or module semantics.
- Java plugins should import Maven or Gradle tasks when a project already has
  `pom.xml` or Gradle build files. A direct Java compiler path is useful as a
  lightweight fallback, not as a duplicate Maven or Gradle model. Maven can be
  embedded in the plugin runtime; Gradle should use Tooling API rather than
  relying on ad hoc parsing.
- Node plugins should import `package.json` scripts and rely on npm, pnpm,
  yarn, or bun for ecosystem behavior. npm script execution can use lifecycle
  libraries such as `@npmcli/package-json` and `@npmcli/run-script`; pnpm and
  yarn should proxy to their real project runtime so workspace, PnP, plugin,
  and pinned-version semantics stay in those ecosystems. A direct TypeScript
  Compiler API path is useful as a fallback, not as a replacement for package
  manager semantics.

This keeps bu1ld closer to a lightweight runtime than a monolithic build
platform. The common interface is the task graph and plugin protocol; the
ecosystem-specific behavior remains owned by the ecosystem.

## Plugin Sources

Build files declare plugins by namespace:

```text
plugin go {
  source = local
  id = "org.bu1ld.go"
  version = "0.1.4"
}

plugin java {
  source = global
  id = "org.bu1ld.java"
  version = "0.1.4"
}

plugin go {
  source = container
  id = "org.bu1ld.go"
  version = "0.1.4"
  image = "ghcr.io/acme/bu1ld-go-plugin:0.1.4"
}

plugin node {
  source = container
  id = "org.bu1ld.node"
  version = "0.1.4"
  image = "ghcr.io/acme/bu1ld-node-plugin:0.1.4"
}
```

Supported runtime sources:

- `builtin`: native Go plugin linked into the CLI.
- `local`: project `.bu1ld/plugins/<id>/<version>`.
- `global`: user `~/.bu1ld/plugins/<id>/<version>`.
- `container`: Docker image started through the official Docker Engine Go API.

For local development, keep `source = local` or `source = global` and set
`path`. The path can point at:

- a plugin executable
- a directory containing `plugin.toml`
- a `plugin.toml` manifest file

When `path` points at a manifest or manifest directory, bu1ld reads the TOML
manifest and starts the declared `binary`. If an exact local/global install path
is missing, bu1ld can also discover plugin manifests under the corresponding
plugin directory.

```text
plugin java {
  source = local
  path = "./plugins/java/build/plugin/plugin.toml"
}
```

Container plugins use the same JSON-RPC protocol, but bu1ld starts an ephemeral
container instead of a host process. The project directory is bind-mounted read
write into the container at `/workspace` by default, and `plugin.exec` work
directories are mapped from host paths into that mount. The image entrypoint or
command must start the plugin JSON-RPC server on stdin/stdout.

```text
plugin go {
  source = container
  id = "org.bu1ld.go"
  version = "0.1.4"
  image = "registry.local/build/bu1ld-go-plugin:0.1.4"
  pull = "missing"
  network = "bridge"
  work_dir = "/workspace"
}
```

Container-specific fields:

- `image`: required container image reference.
- `pull`: optional image pull policy. Supported values are `missing` (default),
  `always`, and `never`.
- `network`: optional Docker network mode such as `bridge`, `host`, or `none`.
  When omitted, Docker's default network mode is used.
- `work_dir`: optional project mount path inside the container. Defaults to
  `/workspace`.

bu1ld does not shell out to `docker`; it uses the Docker Engine Go client
directly. The Docker client reads the normal Docker environment such as
`DOCKER_HOST`, `DOCKER_TLS_VERIFY`, and `DOCKER_CERT_PATH`.

## Manifest

Installed process plugins include `plugin.toml` beside the executable:

```toml
id = "org.example.rust"
namespace = "rust"
version = "0.1.4"
binary = "bu1ld-rust"

[[rules]]
name = "binary"
```

The manifest provides enough information for discovery, `plugins doctor`, lock
file generation, and process startup. The binary path is relative to the
manifest directory unless it is absolute.

## Protocol

Process and container plugins use JSON-RPC 2.0 request/response messages on stdin/stdout.
bu1ld uses `go.lsp.dev/jsonrpc2` with `Content-Length` stream framing, matching
the LSP-style framing supported by libraries such as Eclipse LSP4J JSON-RPC.
Stdout is reserved for protocol frames; logs should go to stderr or a file.

Every plugin `metadata` response must declare `protocol_version = 1` and a
`capabilities` list. The baseline capabilities are `metadata` and `expand`;
plugins add `configure` and/or `execute` when they implement those optional
methods. bu1ld performs a metadata handshake when starting a process plugin and
rejects unsupported protocol versions before evaluating build rules.

Supported capabilities and methods:

- `metadata`: returns plugin ID, namespace, rule schemas, optional config
  fields, and whether the plugin supports automatic configuration.
- `configure`: optional. Converts a plugin config block into task specs.
- `expand`: converts one rule invocation into task specs.
- `execute`: optional. Runs a `plugin.exec` action emitted by the plugin.

The public Go types live in `pkg/pluginapi`. Other languages only need to
match the framing, JSON-RPC method names, and parameter/result JSON shape.

## Task Registration

A plugin can create tasks in two ways:

- Rule expansion: `go.binary app { ... }` calls `expand` for the `binary` rule.
- Auto configuration: a plugin with `auto_configure = true` can read a
  namespace block such as `java { ... }` and return conventional tasks.

Task specs contain the same shape as core tasks:

- `name`
- `deps`
- `inputs`
- `outputs`
- `command`
- `action`

External plugins that need custom execution should emit a `plugin.exec` action:

```json
{
  "kind": "plugin.exec",
  "params": {
    "namespace": "java",
    "action": "compile",
    "params": {
      "srcs": ["src/main/java/**/*.java"],
      "out": "build/classes/java/main"
    }
  }
}
```

During execution bu1ld routes that action back to the resolved plugin process
through `execute`.

## Locks And Doctor

`bu1ld plugins lock` writes `bu1ld.lock` with resolved plugin source,
namespace, ID, version, path/image, and binary checksum when a host executable
is used. Container plugin locks record the image and container options instead
of a host binary checksum. When the lock exists, `plugins doctor` verifies paths
and checksums in addition to manifest validity, image declarations, and protocol
metadata.

`plugins list` and `plugins doctor` are intentionally runtime checks. They
inspect builtin plugins, declared plugins, local installs, global installs, and
manifest-discovered plugins so broken plugin installations fail before a build
needs them.

## Distribution

The runtime plugin system is separate from the distribution registry. A project
can install process plugins from a registry, but after installation the build
uses the local/global/path manifest and binary resolution described here.

See [Plugin Registry](plugin-registry.md) for registry metadata and asset
downloads.
