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
- Apache Maven Resolver for Maven-compatible dependency resolution.
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
declares the `compile`, `resources`, `jar`, and `javadoc` rules.

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
- `processResources`
- `classes`
- `jar`
- `javadoc`
- `sourcesJar`
- `javadocJar`
- `build` unless `register_build = false`

Defaults follow Gradle-like paths:

- sources: `src/main/java/**/*.java`
- source roots: `src/main/java`
- resources: `src/main/resources/**`
- resource roots: `src/main/resources`
- classes: `build/classes/java/main`
- resources output: `build/resources/main`
- javadoc: `build/docs/javadoc`
- jar: `build/libs/<name>.jar`
- sources jar: `build/libs/<name>-sources.jar`
- javadoc jar: `build/libs/<name>-javadoc.jar`
- Maven dependency cache: `~/.m2/repository`
- release: `17`

Supported config fields:

- `name`
- `release`
- `sources`
- `source_roots`
- `resources`
- `resource_roots`
- `classpath`
- `repositories`
- `dependencies`
- `build_dir`
- `classes_dir`
- `resources_dir`
- `javadoc_dir`
- `local_repository`
- `jar`
- `sources_jar`
- `javadoc_jar`
- `register_build`

`sources` and `resources` normally do not need to be declared. The defaults
match the common Gradle project layout.

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
- `repositories`
- `dependencies`
- `local_repository`
- `out`
- `release`
- `deps`
- `inputs`
- `outputs`

Maven dependencies use standard coordinates:

```text
java.compile compileGenerated {
  srcs = ["generated/**/*.java"]
  dependencies = ["com.google.guava:guava:33.5.0-jre"]
}
```

If `repositories` is omitted, the plugin uses Maven Central. Resolver first
checks `local_repository`, which defaults to `~/.m2/repository`, so dependencies
already present in the user's Maven cache are reused without another download.
Set `local_repository = "build/dependency-cache/maven"` when a project-local
cache is preferred. This is only a Maven-layout local repository cache; the
plugin keeps dependency coordinates in bu1ld config and does not import Maven
or Gradle project metadata.

### `java.resources`

Copies resources into an output directory while preserving paths relative to
the configured resource roots.

```text
java.resources processGeneratedResources {
  resources = ["generated/resources/**"]
  resource_roots = ["generated/resources"]
  out = "build/resources/generated"
}
```

Fields:

- `resources`
- `resource_roots`
- `out`
- `deps`
- `inputs`
- `outputs`

### `java.jar`

Writes a jar using Java's jar APIs.

```text
java.jar app {
  roots = ["build/classes/java/main", "build/resources/main"]
  out = "build/libs/app.jar"
}
```

Fields:

- `classes`
- `roots`
- `out`
- `deps`
- `inputs`
- `outputs`

`classes` is retained as a shorthand for a single jar root. New tasks should
prefer `roots`, because application jars, sources jars, and javadoc jars all use
the same packaging action.

### `java.javadoc`

Generates javadoc directly through the JDK `DocumentationTool` API.

```text
java.javadoc apiDocs {
  srcs = ["src/main/java/**/*.java"]
  out = "build/docs/javadoc"
}
```

Fields:

- `srcs`
- `classpath`
- `repositories`
- `dependencies`
- `local_repository`
- `out`
- `release`
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
