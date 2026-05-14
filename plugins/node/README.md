# Node Plugin

The first-party Node plugin is an external bu1ld plugin written in TypeScript.
It speaks the normal bu1ld JSON-RPC protocol over stdin/stdout.

By default it imports `package.json` scripts as bu1ld tasks and executes them
through the matching package-manager runtime. The TypeScript Compiler API
remains available as a direct fallback for small projects without package
scripts.

The npm path uses `@npmcli/package-json` and `@npmcli/run-script`. The pnpm and
yarn paths proxy to the real runtime so workspace, PnP, plugin, and
package-manager-version semantics remain owned by pnpm or yarn.

For local development:

```bash
npm --prefix plugins/node ci
npm --prefix plugins/node run build
```

Projects can then point at `plugins/node/plugin.toml` with `source = local`.
