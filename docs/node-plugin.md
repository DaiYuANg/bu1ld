# Node Plugin

The first-party Node plugin lives in `plugins/node` and is implemented in
TypeScript. It is an external bu1ld plugin that speaks JSON-RPC over
stdin/stdout and can run as a local process or as a container plugin.

The plugin is a Node ecosystem adapter, not a replacement package manager. In
`backend = "auto"` mode it reads `package.json`, imports scripts as bu1ld
tasks, detects npm, pnpm, yarn, or bun from config, `packageManager`, or
lockfiles, and runs those scripts under a bu1ld task boundary. The direct
TypeScript Compiler API path remains as a lightweight fallback when a project
does not already define package scripts.

For npm projects, script execution uses npm's lifecycle libraries:
`@npmcli/package-json` loads the manifest and `@npmcli/run-script` runs the
script with package-bin PATH handling, lifecycle environment, and argument
escaping. pnpm and yarn execution is a runtime proxy instead of a reimplemented
script runner: the plugin prefers project-local package-manager binaries,
honors yarn's `yarnPath`, uses Corepack when `packageManager` pins a version,
and otherwise falls back to the matching runtime on `PATH`. Bun remains a
runtime adapter because its runtime and package manager are distributed
together.

## Local Development

```bash
npm --prefix plugins/node ci
npm --prefix plugins/node run build
```

Then point a project at the local manifest:

```text
plugin node {
  source = local
  id = "org.bu1ld.node"
  version = "0.1.3"
  path = "../../plugins/node/plugin.toml"
}
```

The manifest starts `dist/main.js`; bu1ld launches `.js` plugin binaries with
`node`.

## Auto Configuration

For package-managed projects:

```text
node {
  backend = "auto"
}
```

Given:

```json
{
  "scripts": {
    "typecheck": "tsc --noEmit -p tsconfig.json",
    "build": "tsc -p tsconfig.json"
  }
}
```

the plugin registers:

- `node.typecheck`
- `node.build`

Each task uses `plugin.exec` with action `script`, so bu1ld still sees a typed
task graph, inputs, outputs, and cache boundaries while the actual work remains
inside the Node package manager ecosystem.

For lightweight projects without `package.json` scripts, the compiler fallback
can still be configured directly:

```text
node {
  backend = "compiler"
  srcs = ["src/**/*.ts"]
  out_dir = "dist"
  target = "ES2022"
  module = "CommonJS"
}
```

## Rules

The plugin namespace is `node`.

### `node.script`

```text
node.script build {
  script = "build"
  package_manager = "npm"
}
```

Runs a package script through npm, pnpm, yarn, or bun. If `package_manager` is
not set, detection checks `packageManager`, then lockfiles, then falls back to
`npm`.

### `node.typecheck`

```text
node.typecheck typecheck {
  srcs = ["src/**/*.ts"]
}
```

Runs the TypeScript Compiler API with `noEmit = true`.

### `node.compile`

```text
node.compile compile {
  srcs = ["src/**/*.ts"]
  out_dir = "dist"
}
```

Compiles sources with the TypeScript Compiler API.

### `node.build`

`node.build` is an alias for `node.compile` intended for the fallback compiler
backend. Package-script builds are imported as `node.build` when the project
defines a `build` script.

## Fields

Package adapter fields:

- `backend`: `auto`, `package`, `compiler`, or `none`.
- `import_scripts`: defaults to `true`.
- `package_manager`: `npm`, `pnpm`, `yarn`, or `bun`.
- `scripts`: script names to import; defaults to all `package.json` scripts.
- `task_prefix`: defaults to `node.` for imported scripts.
- `args`: extra arguments for `node.script`.

Compiler fallback fields:

- `deps`, `inputs`, `outputs`
- `srcs`: glob list, defaults to `src/**/*.ts` and `src/**/*.tsx`
- `out_dir`: defaults to `dist`
- `tsconfig`: defaults to `tsconfig.json` when present
- `root_dir`
- `target`, `module`, `module_resolution`, `jsx`
- `strict`, `declaration`, `source_map`, `incremental`
- `no_emit_on_error`, `skip_lib_check`
- `allow_js`, `check_js`
- `lib`, `types`, `type_roots`, `base_url`, `paths`

If `tsconfig.json` is present in compiler mode, the plugin loads it first and
then applies bu1ld field overrides.

## Container

First-party releases publish:

```text
ghcr.io/lyonbrown4d/bu1ld-node-plugin:<version>
```

Container usage:

```text
plugin node {
  source = container
  id = "org.bu1ld.node"
  version = "0.1.3"
  image = "ghcr.io/lyonbrown4d/bu1ld-node-plugin:0.1.3"
}
```
