# bu1ld

`bu1ld` is an early cross-language build tool prototype.

It is intended to be a lightweight build runtime with a unified plugin
interface. The project sits between Makefile-style command lists and
Gradle-style full build systems: it gives tasks a structured graph,
inputs/outputs, cache boundaries, remote execution hooks, plugin discovery, and
artifact handling, while keeping language ecosystems outside the core runtime.

Language plugins should import existing project models and adapt mature
toolchains into bu1ld rather than replace them wholesale. Go tasks use the Go
toolchain, Java tasks can import Maven or Gradle tasks and still keep a light
`javac` fallback, and Node tasks can import package scripts while still keeping
a TypeScript Compiler API fallback for small projects.

The first version includes:

- Cobra command layout: `init`, `build`, `test`, `doctor`, `graph`, `clean`
- Task discovery via `tasks` and targeted graph planning via `graph [task...]`
- Multi-process `cmd/bu1ld`, `cmd/daemon`, `cmd/server`, and `cmd/lsp` executables
- A small `build.bu1ld` DSL
- A basic DSL language server with parse diagnostics, semantic diagnostics, and schema completions
- A plugin registry with embedded, local, HTTP(S), and Git metadata sources
- Builtin `docker`, `archive`, and `git` plugins
- First-party external Go, Java, and Node process plugins
- Monorepo workspace package discovery and package-scoped task ids
- Task graph planning with dependency ordering and cycle detection
- A configuration cache for unchanged build scripts and plugin binaries
- Input fingerprints and a local action cache
- Cached output blobs for declared outputs
- Optional remote action cache served by the coordinator over HTTP
- GoReleaser configuration for cross-platform archives and Linux deb/rpm packages
- A full `arcgolabs/dix` runtime per subcommand
- `arcgolabs/collectionx`, `configx`, `eventx`, and `logx` integration

## Structure

```text
.
├── cmd/
│   ├── bu1ld/
│   │   ├── main.go
│   │   └── root.go
│   ├── daemon/
│   │   └── main.go
│   ├── lsp/
│   │   └── main.go
│   └── server/
│       └── main.go
├── internal/
│   ├── app/
│   │   └── app.go
│   ├── build/
│   ├── cache/
│   ├── config/
│   ├── dsl/
│   ├── engine/
│   ├── events/
│   ├── graph/
│   ├── lsp/
│   ├── plugin/
│   ├── plugins/
│   └── snapshot/
├── pkg/
│   └── pluginapi/
├── plugins/
│   ├── go/
│   │   ├── cmd/
│   │   │   └── bu1ld-go-plugin/
│   │   └── go.mod
│   ├── java/
│   │   ├── build.gradle.kts
│   │   └── src/
│   └── node/
│       ├── package.json
│       └── src/
├── integration/
│   └── vscode/
├── docs/
│   ├── README.md
│   ├── architecture.md
│   ├── plugin-system.md
│   ├── plugin-registry.md
│   ├── go-plugin.md
│   ├── java-plugin.md
│   ├── node-plugin.md
│   ├── remote-cache.md
│   └── releases.md
├── build.bu1ld
├── go.mod
├── go.work
└── README.md
```

## Design Docs

Detailed design notes live under [`docs/`](docs/):

- [Architecture](docs/architecture.md)
- [Plugin System](docs/plugin-system.md)
- [Plugin Registry](docs/plugin-registry.md)
- [Go Plugin](docs/go-plugin.md)
- [Java Plugin](docs/java-plugin.md)
- [Node Plugin](docs/node-plugin.md)
- [Remote Cache](docs/remote-cache.md)
- [Releases](docs/releases.md)
- [Upgrading](docs/upgrading.md)

## DSL

```text
workspace {
  name = "bu1ld"
  default = build
}

git.info version {
  out = "dist/git-info.json"
  include_dirty = true
}

toolchain go {
  version = env("GO_VERSION", "1.26.2")
}

import "tasks/go.bu1ld"
```

Monorepo workspaces can discover package build files from the root build file:

```text
workspace {
  name = "mono"
  packages = ["apps/*", "libs/*"]
}
```

Each package can declare metadata and local tasks in its own `build.bu1ld`:

```text
package {
  name = "apps/api"
  deps = ["libs/core"]
}

task build {
  inputs = ["src/**"]
  outputs = ["dist/**"]
  command = ["go", "build", "./..."]
}
```

Package tasks are exposed as globally unique ids such as `apps/api:build`.
Package dependencies automatically add same-name task dependencies, so
`apps/api:build` depends on `libs/core:build` when both packages define
`build`. Root aggregate tasks can depend on package-qualified task ids by using
string deps such as `deps = ["apps/api:build"]`.

The repository build uses plain tasks so it can bootstrap plugin binaries
without requiring any process plugin to be installed first:

```text
task test {
  inputs = ["build.bu1ld", "go.mod", "go.sum", "**/*.go"]
  command = ["go", "test", "./..."]
}

task build {
  deps = [prepare, test]
  outputs = ["dist/bu1ld"]
  command = ["go", "build", "-o", "dist/bu1ld", "./cmd/bu1ld"]
}
```

Builtin Docker and archive rules use native Go implementations instead of
shelling out to `docker`, `zip`, or `tar` commands:

```text
docker.image app_image {
  context = "."
  dockerfile = "Dockerfile"
  tags = ["example/app:dev"]
  build_args = { VERSION = "dev" }
  load = true
}

archive.zip package_zip {
  deps = [build]
  srcs = ["dist/**"]
  out = "dist/package.zip"
}

archive.tar package_tgz {
  deps = [build]
  srcs = ["dist/**"]
  out = "dist/package.tar.gz"
  gzip = true
}
```

`docker.image` builds through the Docker Go SDK and Docker daemon API. The
first implementation supports local image builds, optional single-platform
selection, build args, target stage selection, and pushing tags after build.
Multi-platform buildx exports are intentionally left for a later Docker
BuildKit iteration.

Block names are typed symbols instead of string labels. Expressions are parsed
directly by `plano`, while imports are resolved relative to the file that
declares them and also support doublestar glob patterns such as
`import "tasks/**/*.bu1ld"`.

Custom tasks can use the same built-in functions and expression context:

```text
task package {
  outputs = ["dist/package"]
  run {
    shell("echo package")
  }
}

task archive {
  deps = [build]
  inputs = ["dist/bu1ld"]
  outputs = ["dist/pack.tgz"]
  run {
    exec("tar", "-czf", "dist/pack.tgz", "dist/bu1ld")
  }
}
```

Recent `plano` syntax is available in `.bu1ld` files, including membership
checks, conditional expressions, and filtered loops:

```text
task package_if_needed {
  let formats = ["zip", "tar"]
  outputs = "zip" in formats ? ["dist/package.zip"] : []
}

task selected_test {
  let packages = ["./...", "./cmd/...", "./internal/..."]
  for pkg in packages where pkg != "./..." {
    command = ["go", "test", pkg]
    break
  }
}
```

`shell(...)` actions are parsed as POSIX shell through `mvdan.cc/sh/v3` before
execution. Use `exec(...)` when the task should avoid shell parsing and pass an
argv list directly to the process runner.

Process plugins can be resolved from local, global, or direct development paths:

```text
plugin rust {
  source = local
  id = "org.bu1ld.rust"
  version = "0.1.4"
}

plugin java {
  source = global
  id = "org.bu1ld.java"
  version = "0.1.4"
}
```

Builtin plugins are native Go implementations linked into the bu1ld binary.
The current builtins are `docker`, `archive`, and `git`. The `git.info`
rule uses go-git to write repository metadata such as HEAD, branch, commit,
dirty state, and remotes into a JSON output.
Local plugins are external process plugins resolved under the project
`.bu1ld/plugins` directory by default. Global plugins are resolved under the
user home `.bu1ld/plugins` directory by default. A `path = "./..."` value can be
used for local plugin development. External process plugins implement the public
`pkg/pluginapi` JSON-RPC protocol over stdin/stdout. When the exact install path
is missing, local and global plugin resolution falls back to file discovery
under the corresponding plugin directory.

Plugins can also run as containers:

```text
plugin go {
  source = container
  id = "org.bu1ld.go"
  version = "0.1.4"
  image = "registry.local/build/bu1ld-go-plugin:0.1.4"
  pull = "missing"
}
```

Container plugins use the same JSON-RPC protocol, but bu1ld starts them through
the official Docker Engine Go API instead of shelling out to `docker`. The
project directory is mounted at `/workspace` by default, and `plugin.exec` work
directories are mapped into that mount.

First-party release images are published to GitHub Container Registry:

- `ghcr.io/lyonbrown4d/bu1ld`
- `ghcr.io/lyonbrown4d/bu1ld-go-plugin`
- `ghcr.io/lyonbrown4d/bu1ld-java-plugin`
- `ghcr.io/lyonbrown4d/bu1ld-node-plugin`

Installed plugins can include a manifest at
`.bu1ld/plugins/<id>/<version>/plugin.toml` or
`~/.bu1ld/plugins/<id>/<version>/plugin.toml`:

```toml
id = "org.bu1ld.rust"
namespace = "rust"
version = "0.1.4"
binary = "bu1ld-rust"

[[rules]]
name = "binary"
```

`bu1ld plugins search`, `bu1ld plugins info`, `bu1ld plugins install`, and
`bu1ld plugins update` operate on the plugin distribution registry. The CLI
embeds the first-party registry entries for `org.bu1ld.go`,
`org.bu1ld.java`, and `org.bu1ld.node`, and projects can override the
registry metadata source with local, HTTP(S), or Git-backed metadata. See
[`docs/plugin-registry.md`](docs/plugin-registry.md) for the registry source
model and TOML schema.

```bash
bu1ld plugins search java
bu1ld plugins info org.bu1ld.go
bu1ld plugins install org.bu1ld.go@0.1.4
bu1ld plugins registry validate ./registry
bu1ld plugins publish ./plugin.toml --asset-url https://downloads.example.com/plugin.tar.gz --os linux --arch amd64 --format tar.gz
BU1LD_PLUGIN_REGISTRY=./registry bu1ld plugins search
BU1LD_PLUGIN_REGISTRY='git+https://example.com/platform/bu1ld-plugins.git?ref=main&path=registry' bu1ld plugins search
```

The first-party Go plugin is external and provides `go.binary`, `go.test`,
`go.generate`, and `go.release`. It can inject `GOCACHEPROG` from bu1ld remote
cache settings and can run GoReleaser directly or through a pinned module
fallback. See [`docs/go-plugin.md`](docs/go-plugin.md) for the full rule model.

```text
plugin go {
  source = local
  id = "org.bu1ld.go"
  version = "0.1.4"
}

go.generate generate {
  out = "build/generated/go"
}

go.test test {
  deps = [generate]
  packages = ["./..."]
}

go.binary build {
  deps = [test]
  main = "./cmd/bu1ld"
  out = "dist/app"
}

go.release snapshot {
  deps = [test]
  mode = "snapshot"
}
```

The first-party Java plugin is written in Java, built with the checked-in Gradle
wrapper, packaged as a JPMS jpackage app image, and owns Java compilation
directly through the `JavaCompiler` API. It supports automatic task
registration for `compileJava`, `classes`, `jar`, and `build`. See
[`docs/java-plugin.md`](docs/java-plugin.md) for packaging, RPC startup,
logging, and rule details.

```text
plugin java {
  source = local
  id = "org.bu1ld.java"
  version = "0.1.4"
}

java {
  name = "app"
  release = "17"
}

java.compile generated {
  srcs = ["generated/**/*.java"]
  out = "build/classes/java/generated"
}
```

The first-party Node plugin is written in TypeScript and adapts the Node
ecosystem into bu1ld. It imports `package.json` scripts as `node.<script>`
tasks, detects npm/pnpm/yarn/bun from project metadata, runs npm scripts
through npm lifecycle libraries, proxies pnpm/yarn/bun to their real runtimes,
and keeps the TypeScript Compiler API as a direct fallback for lightweight
projects. See
[`docs/node-plugin.md`](docs/node-plugin.md) for the rule model.

```text
plugin node {
  source = local
  id = "org.bu1ld.node"
  version = "0.1.4"
}

node {
  backend = "auto"
}
```

Additional first-party plugins can live under `plugins/<name>`. They do not
need to be Go modules; they only need to serve the process protocol and ship a
`plugin.toml` manifest. See [`docs/plugin-system.md`](docs/plugin-system.md).

## Usage

Run or install the released CLI module path:

```bash
go run github.com/lyonbrown4d/bu1ld/cmd/bu1ld@latest --help
go install github.com/lyonbrown4d/bu1ld/cmd/bu1ld@latest
```

```bash
mkdir hello-bu1ld
cd hello-bu1ld
go run ../bu1ld/cmd/bu1ld init
go run ../bu1ld/cmd/bu1ld doctor
go run ../bu1ld/cmd/bu1ld tasks
go run ../bu1ld/cmd/bu1ld build
```

Inside this repository:

```bash
go run ./cmd/bu1ld init --project-dir /tmp/hello-bu1ld
go run ./cmd/bu1ld doctor
go run ./cmd/bu1ld packages
go run ./cmd/bu1ld packages graph
go run ./cmd/bu1ld affected --base main
go run ./cmd/bu1ld build --all :build
go run ./cmd/bu1ld graph
go run ./cmd/bu1ld graph build
go run ./cmd/bu1ld tasks
go run ./cmd/bu1ld test
go run ./cmd/bu1ld build
go run ./cmd/bu1ld clean
go run ./cmd/bu1ld plugins list
go run ./cmd/bu1ld plugins doctor
go run ./cmd/bu1ld plugins lock
go run ./cmd/bu1ld plugins search go
go run ./cmd/bu1ld plugins info org.bu1ld.go
go run ./cmd/bu1ld plugins install org.bu1ld.go@0.1.4
go run ./cmd/bu1ld plugins registry validate
go run ./cmd/bu1ld plugins publish plugins/go/plugin.toml --asset-url https://downloads.example.com/bu1ld-go-plugin.tar.gz --os linux --arch amd64 --format tar.gz
go run ./cmd/daemon status
go run ./cmd/server status
go run ./cmd/lsp stdio
```

Runnable examples live under `examples/archive-basic`, `examples/docker-image`,
`examples/go-project`, `examples/java-project`,
`examples/multilang-monorepo`, and `examples/java-plugin-smoke`. The archive
example is covered by CLI end-to-end tests; the Docker example requires a local
Docker daemon. The plugin examples use local plugin manifests and document the
small bootstrap commands needed before running them.

`plugins list` prints builtin, declared, and manifest-discovered plugins with
source, namespace, resolved path, rules, and status. `plugins doctor` also
checks the local and global plugin directories and returns a non-zero exit when
a plugin is missing, not executable, has an invalid manifest, or cannot answer
the process protocol metadata request.

`plugins lock` writes `bu1ld.lock` with declared plugin source, namespace, id,
version, resolved path, and binary checksum. When `bu1ld.lock` exists,
`plugins doctor` verifies locked plugin paths and checksums.

`plugins search` and `plugins info` read the configured distribution registry,
defaulting to the embedded first-party metadata. External registries can be Git,
local, or HTTP(S) metadata sources; the selected plugin version still controls
the concrete asset URL used for installation. `plugins install` installs a
matching registry asset into `.bu1ld/plugins/<id>/<version>` by default, and
`plugins install --global` targets `~/.bu1ld/plugins/<id>/<version>`.
`plugins update` selects the latest matching version and replaces the installed
copy.

Project configuration is cached under `.bu1ld/cache/config/project.bin`.
`bu1ld` reuses the evaluated task graph when the root build file, imported
files, import glob expansions, environment variables read through `env(...)`,
and external plugin binaries are unchanged. Pass `--no-cache` to bypass both
the configuration cache and build action cache.

The local daemon is optional. Start it with `bu1ld daemon start`; supported
project commands will proxy to it when available and fall back to local
execution when it is not. Pass `--no-daemon` to force in-process execution.
Daemon state lives under `.bu1ld/daemon.json`. See
[`docs/daemon.md`](docs/daemon.md).

Remote action caching uses the same action records and output blobs as the
local cache. The coordinator also exposes Go build-cache resources for
the `bu1ld-go-plugin cacheprog` adapter. See
[`docs/remote-cache.md`](docs/remote-cache.md) for the HTTP API, dotenv-based
LAN configuration, and Go cacheprog behavior.

```bash
go run ./cmd/server coordinator --listen 127.0.0.1:19876
go run ./cmd/bu1ld build --remote-cache-url http://127.0.0.1:19876 --remote-cache-push
go run ./cmd/bu1ld build --remote-cache-url http://127.0.0.1:19876
```

Optional config files are loaded through `configx` from `bu1ld.yaml`, `bu1ld.toml`, `bu1ld.json`, or their `.bu1ld.*` variants.

## Releases

GoReleaser builds the first-party Go executables, including `bu1ld-go-plugin`.
The Go plugin also has an independent GoReleaser config for standalone plugin
releases. The Java plugin is packaged with Gradle/jpackage, and the Node plugin
is packaged with npm plus a Node runtime container image. See
[`docs/releases.md`](docs/releases.md) for the release model.

Local snapshot release:

```bash
goreleaser release --snapshot --clean --skip=publish
```

Tagged releases are handled by `.github/workflows/release.yml`:

```bash
git tag v0.1.4
git push origin v0.1.4
```

## Editor Integrations

The VS Code extension lives under `integration/vscode`. It registers `.bu1ld`
files, provides syntax highlighting and snippets, and starts a bundled language
server over stdio. If no bundled server exists for the current platform, it
falls back to `bu1ld-lsp` from `PATH`; `bu1ld.lsp.path` can override this.

```bash
cd integration/vscode
pnpm install
pnpm run build:server
pnpm run compile
```

Use `pnpm run build:server:all` before packaging a release that should include
all supported platform binaries.

## Test

```bash
go test ./...
go test ./plugins/go/...
./plugins/java/gradlew -p plugins/java check
npm --prefix plugins/node test
go run ./cmd/bu1ld build --no-cache java_plugin_verify
```
