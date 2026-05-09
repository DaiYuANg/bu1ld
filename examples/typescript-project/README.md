# TypeScript Project

This example uses the first-party TypeScript plugin directly through the
TypeScript Compiler API. There is no project `package.json`; bu1ld owns the
typecheck and compile tasks.

From the repository root:

```bash
npm --prefix plugins/typescript ci
npm --prefix plugins/typescript run build
go run ./cmd/cli --project-dir examples/typescript-project build
```
