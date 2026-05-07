# bu1ld

`bu1ld` is an early cross-language build tool prototype.

The first version includes:

- Cobra command layout: `init`, `build`, `test`, `doctor`, `graph`, `clean`
- Task discovery via `tasks` and targeted graph planning via `graph [task...]`
- Multi-process `cmd/cli`, `cmd/daemon`, `cmd/server`, and `cmd/lsp` executables
- A small `build.bu1ld` DSL
- A basic DSL language server with parse diagnostics, semantic diagnostics, and schema completions
- A plugin registry with builtin, local, and global plugin sources
- Builtin `docker`, `archive`, and `git` plugins
- First-party external Go and Java process plugins
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
│   ├── cli/
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
│   └── java/
│       ├── build.gradle.kts
│       └── src/
├── integration/
│   └── vscode/
├── build.bu1ld
├── go.mod
├── go.work
└── README.md
```

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
`build`.

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
  command = ["go", "build", "-o", "dist/bu1ld", "./cmd/cli"]
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

Plugins can come from three sources:

```text
plugin rust {
  source = local
  id = "org.bu1ld.rust"
  version = "0.1.0"
}

plugin java {
  source = global
  id = "org.bu1ld.java"
  version = "0.1.0"
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

Installed plugins can include a manifest at
`.bu1ld/plugins/<id>/<version>/plugin.toml` or
`~/.bu1ld/plugins/<id>/<version>/plugin.toml`:

```toml
id = "org.bu1ld.rust"
namespace = "rust"
version = "0.1.0"
binary = "bu1ld-rust"

[[rules]]
name = "binary"
```

The first-party Go plugin is external. Build it with:

```bash
go build -C plugins/go -o ../../.bu1ld/plugins/org.bu1ld.go/0.1.0/bu1ld-go-plugin ./cmd/bu1ld-go-plugin
```

On Windows, use `bu1ld-go-plugin.exe` for both the output file and manifest
binary.

Then install a manifest beside the binary:

```toml
id = "org.bu1ld.go"
namespace = "go"
version = "0.1.0"
binary = "bu1ld-go-plugin"

[[rules]]
name = "binary"

[[rules]]
name = "test"
```

Projects can opt into it with:

```text
plugin go {
  source = local
  id = "org.bu1ld.go"
  version = "0.1.0"
}

go.test test {
  packages = ["./..."]
}

go.binary build {
  deps = [test]
  main = "./cmd/cli"
  out = "dist/app"
}

go.release snapshot {
  deps = [test]
  mode = "snapshot"
}
```

The Go plugin executes `go.binary` and `go.test` through `plugin.exec`, so it
can inject Go toolchain environment settings. When `BU1LD_REMOTE_CACHE__URL` is
configured, the plugin derives `GOCACHEPROG` automatically:

```dotenv
BU1LD_REMOTE_CACHE__URL=http://192.168.1.10:19876
BU1LD_REMOTE_CACHE__PULL=true
BU1LD_REMOTE_CACHE__PUSH=true
```

That starts `bu1ld-go-cacheprog --remote-cache-url <url>` as Go's
`GOCACHEPROG`. The adapter speaks Go's stdin/stdout cacheprog protocol locally
and stores action/output records in the bu1ld coordinator over HTTP. Set
`BU1LD_GO__CACHEPROG` or an individual rule's `cacheprog = "..."` field to
override the generated command.

`go.release` embeds GoReleaser orchestration in the plugin. It prefers a local
`goreleaser` binary when one is on `PATH`; otherwise it runs the pinned module
fallback `go run github.com/goreleaser/goreleaser/v2@v2.15.4 ...`. The default
mode is a local snapshot release:

```text
go.release snapshot {
  mode = "snapshot"
  config = ".goreleaser.yaml"
}
```

Set `mode = "release"` for tagged release arguments, or use `args = [...]` for
full control over the GoReleaser command line.

The first-party Java plugin is written in Java, built with Gradle, uses Jackson
for protocol JSON, SLF4J and Logback for logging, Avaje Inject for dependency
injection, Commons Lang and Commons IO for small utilities, Guava for immutable
collections and classpath handling, and the FreeFair Lombok Gradle plugin for
Lombok wiring. Its Gradle build uses `gradle/plugin.versions.toml` for version
catalog and the checked-in Gradle wrapper only to build the plugin itself. Java
project builds are native bu1ld plugin actions: `compileJava` calls the
`JavaCompiler` API directly and `jar` writes the archive through Java's jar APIs.
Packaging uses the
`org.beryx.jlink` plugin for `jpackageImage`; this path only builds an app-image
artifact (no MSI/DMG/DEB/RPM installers):
`skipInstaller = true`.
The plugin is now packaged as a JPMS module (`org.bu1ld.plugins.java`) and
`jlink` uses a trimmed runtime image with `--strip-debug`, `--compress 2`,
`--no-header-files`, `--no-man-pages`, and `--strip-native-commands`. Build it with:

```bash
./plugins/java/gradlew -p plugins/java installBu1ldPlugin
```
The Gradle wrapper jar is checked in under
`plugins/java/gradle/wrapper/gradle-wrapper.jar`, so this does not require a
locally installed Gradle.

This produces plugin artifacts under
`.bu1ld/plugins/org.bu1ld.java/0.1.0/`. A local install can copy that folder to
`.bu1ld/plugins/org.bu1ld.java/0.1.0/`.

Inside this repository, the full Java plugin smoke path is:

```bash
go run ./cmd/cli build --no-cache java_plugin_verify
```

That task runs the Java plugin Gradle checks, builds and installs the jpackage
app image, then builds `examples/java-plugin-smoke` through the external Java
plugin. The example uses Gradle-style defaults such as `src/main/java`,
`build/classes/java/main`, and `build/libs/<name>.jar`.

The Java RPC server starts from the jpackage launcher generated for
`org.bu1ld.plugins.java.Bu1ldJavaPlugin`. The CLI resolves `plugin.toml`, starts
the configured binary as an external process, and exchanges line-delimited JSON
messages over stdin/stdout. The Java main method creates an Avaje `BeanScope`,
gets `Server`, and calls `serve(System.in, System.out)`; the server dispatches
`metadata`, `configure`, `expand`, and `execute` requests until stdin closes.
Plugin logs are written through Logback to stderr and to
`.bu1ld/logs/java-plugin.log` by default. Set `BU1LD_PLUGIN_LOG_DIR` to move the
file logs and `BU1LD_PLUGIN_LOG_LEVEL` to adjust `org.bu1ld.plugins.java`
verbosity. The Java launcher installs `jul-to-slf4j`, so JUL-based framework
logs flow into the same Logback pipeline while stdout remains reserved for
JSON-RPC.

Plugins can register tasks during project configuration. The Java plugin opts
into this with `auto_configure`, so a project only needs a plugin declaration
and an optional `java { ... }` block to get `compileJava`, `classes`, `jar`, and
`build` tasks.

Projects can opt into it with:

```text
plugin java {
  source = local
  id = "org.bu1ld.java"
  version = "0.1.0"
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

Additional first-party plugins can live under `plugins/<name>`. They do not
need to be Go modules; they only need to serve the line-delimited
`pkg/pluginapi` JSON-RPC protocol (`metadata`, optional `configure`, `expand`,
and optional `execute`) and ship a `plugin.toml` manifest.

## Usage

```bash
mkdir hello-bu1ld
cd hello-bu1ld
go run ../bu1ld/cmd/cli init
go run ../bu1ld/cmd/cli doctor
go run ../bu1ld/cmd/cli tasks
go run ../bu1ld/cmd/cli build
```

Inside this repository:

```bash
go run ./cmd/cli init --project-dir /tmp/hello-bu1ld
go run ./cmd/cli doctor
go run ./cmd/cli packages
go run ./cmd/cli packages graph
go run ./cmd/cli affected --base main
go run ./cmd/cli build --all :build
go run ./cmd/cli graph
go run ./cmd/cli graph build
go run ./cmd/cli tasks
go run ./cmd/cli test
go run ./cmd/cli build
go run ./cmd/cli clean
go run ./cmd/cli plugins list
go run ./cmd/cli plugins doctor
go run ./cmd/cli plugins lock
go run ./cmd/daemon status
go run ./cmd/server status
go run ./cmd/lsp stdio
```

Runnable examples live under `examples/archive-basic`, `examples/docker-image`,
and `examples/java-plugin-smoke`. The archive example is covered by CLI
end-to-end tests; the Docker example requires a local Docker daemon.

`plugins list` prints builtin, declared, and manifest-discovered plugins with
source, namespace, resolved path, rules, and status. `plugins doctor` also
checks the local and global plugin directories and returns a non-zero exit when
a plugin is missing, not executable, has an invalid manifest, or cannot answer
the process protocol metadata request.

`plugins lock` writes `bu1ld.lock` with declared plugin source, namespace, id,
version, resolved path, and binary checksum. When `bu1ld.lock` exists,
`plugins doctor` verifies locked plugin paths and checksums.

Project configuration is cached under `.bu1ld/cache/config/project.bin`.
`bu1ld` reuses the evaluated task graph when the root build file, imported
files, import glob expansions, environment variables read through `env(...)`,
and external plugin binaries are unchanged. Pass `--no-cache` to bypass both
the configuration cache and build action cache.

Remote action caching uses the same action records and output blobs as the
local cache, exposed through a small HTTP CAS served by the coordinator:

```bash
go run ./cmd/server coordinator --listen 127.0.0.1:19876
go run ./cmd/cli build --remote-cache-url http://127.0.0.1:19876 --remote-cache-push
go run ./cmd/cli build --remote-cache-url http://127.0.0.1:19876
```

Remote pulls are enabled when `--remote-cache-url` is set. Remote pushes are
opt-in through `--remote-cache-push`, so regular builds do not publish outputs
unless requested.

The coordinator also exposes Go build-cache resources for `bu1ld-go-cacheprog`:

```text
GET/HEAD/PUT /v1/go/cache/actions/{actionID}
GET/HEAD/PUT /v1/go/cache/outputs/{outputID}
```

`actionID` and `outputID` are the 64-character hex forms of Go's cacheprog
`ActionID` and `OutputID`; output bodies are stored in the same content-addressed
blob store used by bu1ld action cache.

For LAN setups, `BU1LD_` environment variables can hold the same settings.
`configx` loads dotenv values before normal environment variables, and
`bu1ld.toml` can choose an environment-specific dotenv file:

```toml
env = "lan"
```

With `env = "lan"`, bu1ld loads `.env.lan.local`, `.env.lan`, `.env.local`,
and `.env` from `--project-dir`:

```dotenv
BU1LD_SERVER__COORDINATOR__LISTEN_ADDR=0.0.0.0:19876
BU1LD_REMOTE_CACHE__URL=http://192.168.1.10:19876
BU1LD_REMOTE_CACHE__PULL=true
BU1LD_REMOTE_CACHE__PUSH=false
```

Optional config files are loaded through `configx` from `bu1ld.yaml`, `bu1ld.toml`, `bu1ld.json`, or their `.bu1ld.*` variants.

## Releases

GoReleaser builds the first-party Go executables:

- `bu1ld`
- `bu1ld-server`
- `bu1ld-daemon`
- `bu1ld-lsp`
- `bu1ld-go-cacheprog`
- `bu1ld-go-plugin`

The Go plugin is packaged as its own archive and Linux package with
`plugins/go/plugin.toml` included beside the plugin binary. It also has an
independent GoReleaser config at `plugins/go/.goreleaser.yaml` for standalone
plugin releases:

```bash
cd plugins/go
goreleaser release --snapshot --clean --skip=publish
```

Local snapshot release:

```bash
goreleaser release --snapshot --clean --skip=publish
```

Tagged releases are handled by `.github/workflows/release.yml`:

```bash
git tag v0.1.0-alpha.1
git push origin v0.1.0-alpha.1
```

The release workflow runs Go tests, Go plugin tests, the Java plugin Gradle
check, then publishes GoReleaser archives, checksums, and Linux `deb`/`rpm`
packages. Java plugin packaging remains Gradle/jpackage-based through
`plugins/java/gradlew -p plugins/java installBu1ldPlugin`.

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
go run ./cmd/cli build --no-cache java_plugin_verify
```
