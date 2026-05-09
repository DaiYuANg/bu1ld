# TypeScript Plugin

The first-party TypeScript plugin lives in `plugins/typescript` and is itself
implemented in TypeScript. It is still a normal external bu1ld plugin: the
runtime speaks JSON-RPC over stdin/stdout and can run as a local process or as a
container plugin.

The plugin is intentionally not a `package.json` script wrapper. It uses the
TypeScript Compiler API directly for type checking and compilation. Project
`package.json` files can exist for applications, but bu1ld task registration is
driven by `typescript { ... }` configuration and rule fields.

## Local Development

```bash
npm --prefix plugins/typescript ci
npm --prefix plugins/typescript run build
```

Then point a project at the local manifest:

```text
plugin typescript {
  source = local
  id = "org.bu1ld.typescript"
  version = "0.1.3"
  path = "../../plugins/typescript/plugin.toml"
}
```

The manifest starts `dist/main.js`; bu1ld launches `.js` plugin binaries with
`node`.

## Auto Configuration

The plugin supports a top-level config block:

```text
typescript {
  srcs = ["src/**/*.ts"]
  out_dir = "dist"
  target = "ES2022"
  module = "CommonJS"
}
```

When the project contains TypeScript sources or an explicit config block, the
plugin registers:

- `typecheck`: runs the compiler with `noEmit`.
- `build`: compiles sources to `out_dir` and depends on `typecheck`.

Set `typecheck = false` or `build = false` to skip either task.

## Rules

The plugin namespace is `typescript`.

### `typescript.typecheck`

```text
typescript.typecheck typecheck {
  srcs = ["src/**/*.ts"]
}
```

Runs the compiler with `noEmit = true`.

### `typescript.compile`

```text
typescript.compile compile {
  srcs = ["src/**/*.ts"]
  out_dir = "dist"
}
```

Compiles sources with the TypeScript Compiler API.

### `typescript.build`

`typescript.build` is an alias for `typescript.compile` intended for the common
build task name.

## Fields

Common fields:

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

If `tsconfig.json` is present, the plugin loads it first and then applies bu1ld
field overrides.

## Container

First-party releases publish:

```text
ghcr.io/daiyuang/bu1ld-typescript-plugin:<version>
```

Container usage:

```text
plugin typescript {
  source = container
  id = "org.bu1ld.typescript"
  version = "0.1.3"
  image = "ghcr.io/daiyuang/bu1ld-typescript-plugin:0.1.3"
}
```
