# go-project

Small Go project using the first-party external Go plugin. It runs
`go generate`, `go test`, and `go build`, with generated files written under
`build/generated/go`.

From the repository root, prepare the local plugin:

```bash
go build -C plugins/go -o bu1ld-go-plugin ./cmd/bu1ld-go-plugin
```

On Windows:

```powershell
go build -C plugins/go -o bu1ld-go-plugin.exe ./cmd/bu1ld-go-plugin
```

Then build the project:

```bash
go run ./cmd/cli --project-dir examples/go-project build
```
