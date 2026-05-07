# Releases

bu1ld uses GoReleaser for Go binaries and Gradle/jpackage for the Java plugin.
The release model keeps first-party plugins visible as normal external plugins
instead of hiding them inside the core CLI.

## Root GoReleaser

The root `.goreleaser.yaml` builds:

- `bu1ld`
- `bu1ld-server`
- `bu1ld-daemon`
- `bu1ld-lsp`
- `bu1ld-go-cacheprog`
- `bu1ld-go-plugin`

It produces cross-platform archives, checksums, and Linux packages. The Go
plugin package includes `plugins/go/plugin.toml` beside the plugin binary so it
can be installed as `org.bu1ld.go`.

Local snapshot:

```bash
goreleaser release --snapshot --clean --skip=publish
```

## Go Plugin Standalone Release

The Go plugin also has an independent config at `plugins/go/.goreleaser.yaml`.
Use it when publishing or testing only the Go plugin:

```bash
cd plugins/go
goreleaser release --snapshot --clean --skip=publish
```

The plugin's `go.release` rule can also invoke GoReleaser from a bu1ld project.
It prefers a local `goreleaser` binary and falls back to:

```bash
go run github.com/goreleaser/goreleaser/v2@v2.15.4 ...
```

See [Go Plugin](go-plugin.md) for rule fields.

## Java Plugin Packaging

The Java plugin is packaged by Gradle:

```bash
./plugins/java/gradlew -p plugins/java installBu1ldPlugin
```

This builds a JPMS jpackage app image, writes `plugin.toml`, and installs it
under `.bu1ld/plugins/org.bu1ld.java/0.1.0/`. The Java plugin does not currently
publish MSI, DMG, DEB, or RPM installers; the app image is the plugin artifact.

See [Java Plugin](java-plugin.md) for the packaging model.

## GitHub Release Flow

Tagged releases are handled by `.github/workflows/release.yml`.

```bash
git tag v0.1.0
git push origin v0.1.0
```

The workflow runs:

- root Go tests
- Go plugin tests
- Java plugin Gradle checks
- root GoReleaser publish

GitHub Release assets are produced from GoReleaser output. Java plugin
packaging remains Gradle/jpackage-based and can be attached to a release in a
later workflow step when the plugin distribution format is finalized.

## Registry Update

After a release, update the plugin registry metadata with the new plugin version
and asset URLs. The registry should contain metadata only; the actual asset URL
can point at GitHub Release, an object store, an internal HTTP server, or any
other artifact location.

See [Plugin Registry](plugin-registry.md).
