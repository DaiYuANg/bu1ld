# Node Project

This example uses the first-party Node plugin as an ecosystem adapter. The
plugin reads `package.json`, imports package scripts as bu1ld tasks, and runs
them through the detected package manager.

From the repository root:

```bash
npm --prefix plugins/node ci
npm --prefix plugins/node run build
npm --prefix examples/node-project ci
go run ./cmd/cli --project-dir examples/node-project build
```
