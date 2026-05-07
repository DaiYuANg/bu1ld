# Java Plugin

The first-party Java plugin is written in Java and packaged as an external
process plugin. It is designed to own Java build behavior directly instead of
adapting Maven or Gradle project builds.

The Gradle project under `plugins/java` only builds and packages the plugin
itself. Java projects that use the plugin are compiled by bu1ld plugin tasks.

## Build Stack

The plugin build uses:

- Gradle wrapper checked into `plugins/java/gradle/wrapper`.
- Version catalog at `plugins/java/gradle/plugin.versions.toml`.
- Eclipse LSP4J JSON-RPC for the process plugin protocol.
- Avaje Inject for dependency injection.
- Apache Commons Lang and Commons IO for utilities.
- Guava for immutable collections and classpath handling.
- FreeFair Lombok Gradle plugin for Lombok wiring.
- `org.beryx.jlink` for JPMS runtime trimming and `jpackageImage`.

Build and install locally:

```bash
./plugins/java/gradlew -p plugins/java installBu1ldPlugin
```

On Windows:

```powershell
.\plugins\java\gradlew.bat -p plugins/java installBu1ldPlugin
```

The task writes the plugin under
`.bu1ld/plugins/org.bu1ld.java/0.1.0/`.

For local development without installing, point a project directly at the
generated manifest:

```text
plugin java {
  source = local
  path = "./plugins/java/build/plugin/plugin.toml"
}
```

The manifest's `binary` field points back to the jpackage app image generated
by `assemble`.

## Packaging

The plugin is a JPMS module named `org.bu1ld.plugins.java`. The Gradle build
creates a jpackage app image and skips platform installers:

```kotlin
skipInstaller = true
```

The trimmed runtime uses:

- `--strip-debug`
- `--compress 2`
- `--no-header-files`
- `--no-man-pages`
- `--strip-native-commands`

The generated launcher starts
`org.bu1ld.plugins.java.Bu1ldJavaPlugin`, and the generated `plugin.toml`
declares the `compile` and `jar` rules.

## Runtime Model

The CLI resolves `plugin.toml`, starts the configured launcher as an external
process, and exchanges JSON-RPC 2.0 messages over stdin/stdout. The Go side
uses `go.lsp.dev/jsonrpc2` stream framing; the Java side uses Eclipse LSP4J
JSON-RPC `Launcher`, so both sides use `Content-Length` framed messages.

The Java main method:

1. Creates an Avaje `BeanScope`.
2. Resolves `Server`.
3. Calls `serve(System.in, System.out)`.

The server dispatches:

- `metadata`
- `configure`
- `expand`
- `execute`

until stdin closes. Stdout is reserved for protocol messages.

## Build Rules

The namespace is `java`.

### Auto Configuration

The plugin returns `auto_configure = true` from metadata. A project can declare
the plugin and an optional `java { ... }` block:

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
```

The plugin registers:

- `compileJava`
- `classes`
- `jar`
- `build` unless `register_build = false`

Defaults follow Gradle-like paths:

- sources: `src/main/java/**/*.java`
- classes: `build/classes/java/main`
- jar: `build/libs/<name>.jar`
- release: `17`

Supported config fields:

- `name`
- `release`
- `srcs`
- `classpath`
- `build_dir`
- `classes_dir`
- `jar`
- `register_build`

### `java.compile`

Compiles Java sources by directly calling the `JavaCompiler` API.

```text
java.compile generated {
  srcs = ["generated/**/*.java"]
  out = "build/classes/java/generated"
}
```

Fields:

- `srcs`
- `classpath`
- `out`
- `release`
- `deps`
- `inputs`
- `outputs`

### `java.jar`

Writes a jar using Java's jar APIs.

```text
java.jar app {
  classes = "build/classes/java/main"
  out = "build/libs/app.jar"
}
```

Fields:

- `classes`
- `out`
- `deps`
- `inputs`
- `outputs`

## Verification

The repository smoke path builds the plugin, installs the jpackage app image,
and builds `examples/java-plugin-smoke` through the external Java plugin:

```bash
go run ./cmd/cli build --no-cache java_plugin_verify
```

The plugin Gradle check can also be run directly:

```bash
./plugins/java/gradlew -p plugins/java check
```
