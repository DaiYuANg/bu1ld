# multilang-monorepo

Small monorepo with one Java library package and one Go CLI package. The root
workspace discovers both packages and the Go package declares a package-level
dependency on the Java package, so `apps/go-cli:build` runs after
`libs/java-greeting:build`.

From the repository root, prepare both local plugins:

```bash
go build -C plugins/go -o bu1ld-go-plugin ./cmd/bu1ld-go-plugin
./plugins/java/gradlew -p plugins/java assemble
```

On Windows:

```powershell
go build -C plugins/go -o bu1ld-go-plugin.exe ./cmd/bu1ld-go-plugin
.\plugins\java\gradlew.bat -p plugins/java assemble
```

Then build the workspace:

```bash
go run ./cmd/cli --project-dir examples/multilang-monorepo build
```

To run all package build tasks explicitly:

```bash
go run ./cmd/cli --project-dir examples/multilang-monorepo build --all :build
```
