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
under `.bu1ld/plugins/org.bu1ld.java/0.1.1/`. The Java plugin does not currently
publish MSI, DMG, DEB, or RPM installers; the app image is the plugin artifact.

See [Java Plugin](java-plugin.md) for the packaging model.

## GitHub Release Flow

Tagged releases are handled by `.github/workflows/release.yml`.

```bash
git tag v0.1.1
git push origin v0.1.1
```

The workflow runs:

- root Go tests
- Go plugin tests
- Java plugin Gradle checks
- root GoReleaser publish
- Java plugin jpackage app-image builds on Linux, Windows, and macOS runners
- release asset checksum verification after all uploads

Pull requests and normal branch pushes are covered by `.github/workflows/ci.yml`
with a Linux/macOS/Windows matrix for Go tests, Go plugin tests, and Java plugin
checks.

Go plugin release assets are produced by GoReleaser. Java plugin release assets
are produced after GoReleaser creates the GitHub Release: each platform job runs
`plugins/java assemble`, copies the jpackage app image contents and
`plugin.toml` into one staging directory, archives that directory, and uploads
the result to the same release.

Java plugin asset names follow:

- `bu1ld-java-plugin_<version>_linux_amd64.tar.gz`
- `bu1ld-java-plugin_<version>_darwin_amd64.tar.gz`
- `bu1ld-java-plugin_<version>_darwin_arm64.tar.gz`
- `bu1ld-java-plugin_<version>_windows_amd64.zip`

The archive root contains `plugin.toml` plus the app image contents. This means
registry installation and local `path = ".../plugin.toml"` development both use
the same manifest-driven layout.

Each Java plugin asset is uploaded with a sibling `.sha256` file. GoReleaser
publishes `checksums.txt` for Go-built assets. The release workflow downloads
the final GitHub Release assets and runs:

```bash
scripts/verify-release-assets.sh dist/release
```

The script verifies `checksums.txt` plus every `*.sha256` file in the release
directory.

## Registry Update

After a release, update the plugin registry metadata with the new plugin version
and asset URLs. The registry should contain metadata only; the actual asset URL
can point at GitHub Release, an object store, an internal HTTP server, or any
other artifact location. The first-party embedded registry already follows this
shape for `org.bu1ld.go` and `org.bu1ld.java`.

See [Plugin Registry](plugin-registry.md).

## Upgrade Notes

Version upgrade guidance lives in [Upgrading](upgrading.md). Before publishing a
tag, add user-facing compatibility notes there, including CLI changes, plugin
manifest changes, cache protocol changes, and registry metadata changes.
