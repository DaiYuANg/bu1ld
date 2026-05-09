# TypeScript Plugin

The first-party TypeScript plugin is an external bu1ld plugin written in
TypeScript. It speaks the normal bu1ld JSON-RPC protocol over stdin/stdout and
uses the TypeScript Compiler API directly for type checking and compilation.

The plugin does not read project `package.json` scripts to create bu1ld tasks.
Build behavior is declared through bu1ld fields or `tsconfig.json`.

For local development:

```bash
npm --prefix plugins/typescript ci
npm --prefix plugins/typescript run build
```

Projects can then point at `plugins/typescript/plugin.toml` with `source =
local`.
