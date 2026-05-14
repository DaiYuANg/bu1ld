import assert from "node:assert/strict";
import { chmodSync, existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync, mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";

import { configure, execute, expand, metadata } from "./plugin";

test("metadata exposes Node ecosystem and compiler rules", () => {
  const result = metadata();
  assert.equal(result.id, "org.bu1ld.node");
  assert.equal(result.namespace, "node");
  assert.equal(result.auto_configure, true);
  assert.deepEqual(
    result.rules.map((rule) => rule.name),
    ["script", "typecheck", "compile", "build"],
  );
});

test("script rule expands to a package manager plugin.exec task", () => {
  const [task] = expand({
    namespace: "node",
    rule: "script",
    target: "lint",
    fields: {
      script: "lint",
      package_manager: "npm",
    },
  });

  assert.equal(task.name, "lint");
  assert.equal(task.action?.kind, "plugin.exec");
  assert.equal(task.action?.params.namespace, "node");
  assert.equal(task.action?.params.action, "script");
  assert.deepEqual(task.action?.params.params, {
    manager: "npm",
    script: "lint",
    args: [],
  });
});

test("build rule expands to a compiler plugin.exec task", () => {
  const [task] = expand({
    namespace: "node",
    rule: "build",
    target: "build",
    fields: {},
  });

  assert.equal(task.name, "build");
  assert.deepEqual(task.outputs, ["dist"]);
  assert.equal(task.action?.kind, "plugin.exec");
  assert.equal(task.action?.params.namespace, "node");
  assert.equal(task.action?.params.action, "compile");
});

test("configure imports package scripts by default", () => {
  const root = mkdtempSync(path.join(tmpdir(), "bu1ld-node-plugin-package-"));
  const previous = process.env.BU1LD_PROJECT_DIR;
  process.env.BU1LD_PROJECT_DIR = root;
  try {
    writeFileSync(path.join(root, "package-lock.json"), "{}\n");
    writeFileSync(path.join(root, "package.json"), JSON.stringify({
      scripts: {
        typecheck: "tsc --noEmit",
        build: "tsc",
      },
    }, null, 2));

    const tasks = configure({ namespace: "node", fields: {} });
    assert.deepEqual(tasks.map((task) => task.name), ["node.typecheck", "node.build"]);
    const build = tasks.find((task) => task.name === "node.build");
    assert.equal(build?.action?.params.action, "script");
    assert.deepEqual(build?.action?.params.params, {
      manager: "npm",
      script: "build",
      args: [],
    });
  } finally {
    if (previous === undefined) {
      delete process.env.BU1LD_PROJECT_DIR;
    } else {
      process.env.BU1LD_PROJECT_DIR = previous;
    }
    rmSync(root, { recursive: true, force: true });
  }
});

test("configure detects pnpm and yarn package managers", () => {
  const cases = [
    {
      name: "pnpm packageManager metadata",
      packageManager: "pnpm@10.0.0",
      lockfile: undefined,
      expected: "pnpm",
    },
    {
      name: "yarn lockfile",
      packageManager: undefined,
      lockfile: "yarn.lock",
      expected: "yarn",
    },
  ];

  for (const item of cases) {
    const root = mkdtempSync(path.join(tmpdir(), `bu1ld-node-plugin-${item.expected}-detect-`));
    const previous = process.env.BU1LD_PROJECT_DIR;
    process.env.BU1LD_PROJECT_DIR = root;
    try {
      if (item.lockfile !== undefined) {
        writeFileSync(path.join(root, item.lockfile), "");
      }
      writeFileSync(path.join(root, "package.json"), JSON.stringify({
        packageManager: item.packageManager,
        scripts: {
          build: "node build.js",
        },
      }, null, 2));

      const [task] = configure({ namespace: "node", fields: {} });
      const params = task.action?.params.params as Record<string, unknown>;
      assert.equal(task.name, "node.build", item.name);
      assert.equal(params.manager, item.expected, item.name);
    } finally {
      if (previous === undefined) {
        delete process.env.BU1LD_PROJECT_DIR;
      } else {
        process.env.BU1LD_PROJECT_DIR = previous;
      }
      rmSync(root, { recursive: true, force: true });
    }
  }
});

test("configure creates compiler tasks without package.json", () => {
  const root = mkdtempSync(path.join(tmpdir(), "bu1ld-ts-plugin-"));
  const previous = process.env.BU1LD_PROJECT_DIR;
  process.env.BU1LD_PROJECT_DIR = root;
  try {
    mkdirSync(path.join(root, "src"));
    writeFileSync(path.join(root, "src", "index.ts"), "export const message: string = 'hello';\n");

    const tasks = configure({ namespace: "node", fields: {} });
    assert.deepEqual(tasks.map((task) => task.name), ["typecheck", "build"]);
    assert.deepEqual(tasks.find((task) => task.name === "build")?.deps, ["typecheck"]);
  } finally {
    if (previous === undefined) {
      delete process.env.BU1LD_PROJECT_DIR;
    } else {
      process.env.BU1LD_PROJECT_DIR = previous;
    }
    rmSync(root, { recursive: true, force: true });
  }
});

test("execute runs npm scripts through npm lifecycle libraries", async () => {
  const root = mkdtempSync(path.join(tmpdir(), "bu1ld-node-plugin-npm-script-"));
  try {
    mkdirSync(path.join(root, "dist"));
    writeFileSync(path.join(root, "write-output.js"), [
      "const fs = require('node:fs');",
      "const agent = process.env.npm_config_user_agent || '';",
      "const args = process.argv.slice(2).join(',');",
      "fs.writeFileSync('dist/out.txt', `${agent}:${args}`);",
    ].join("\n"));
    writeFileSync(path.join(root, "package.json"), JSON.stringify({
      scripts: {
        build: "node write-output.js",
      },
    }, null, 2));

    const result = await execute({
      namespace: "node",
      action: "script",
      work_dir: root,
      params: {
        manager: "npm",
        script: "build",
        args: ["--from-bu1ld"],
      },
    });

    assert.match(result.output ?? "", /build/);
    assert.equal(readFileSync(path.join(root, "dist", "out.txt"), "utf8"), "npm/bu1ld:--from-bu1ld");
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("execute proxies pnpm scripts through a project-local package manager runtime", async () => {
  const root = mkdtempSync(path.join(tmpdir(), "bu1ld-node-plugin-pnpm-runtime-"));
  try {
    writeProjectLocalPackageManager(root, "pnpm");
    writeFileSync(path.join(root, "package.json"), JSON.stringify({
      scripts: {
        build: "node write-output.js",
      },
    }, null, 2));

    const result = await execute({
      namespace: "node",
      action: "script",
      work_dir: root,
      params: {
        manager: "pnpm",
        script: "build",
        args: ["--from-bu1ld"],
      },
    });

    assert.match(result.output ?? "", /pnpm runtime/);
    assert.equal(readFileSync(path.join(root, "dist", "out.txt"), "utf8"), "run build --from-bu1ld");
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("execute proxies yarn scripts through yarnPath", async () => {
  const root = mkdtempSync(path.join(tmpdir(), "bu1ld-node-plugin-yarn-runtime-"));
  try {
    const yarnRuntime = path.join(root, ".yarn", "releases", "yarn-test.cjs");
    mkdirSync(path.dirname(yarnRuntime), { recursive: true });
    writeRuntimeScript(yarnRuntime, "yarn");
    writeFileSync(path.join(root, ".yarnrc.yml"), "yarnPath: .yarn/releases/yarn-test.cjs\n");
    writeFileSync(path.join(root, "package.json"), JSON.stringify({
      packageManager: "yarn@4.0.0",
      scripts: {
        build: "node write-output.js",
      },
    }, null, 2));

    const result = await execute({
      namespace: "node",
      action: "script",
      work_dir: root,
      params: {
        manager: "yarn",
        script: "build",
        args: ["--from-bu1ld"],
      },
    });

    assert.match(result.output ?? "", /yarn runtime/);
    assert.equal(readFileSync(path.join(root, "dist", "out.txt"), "utf8"), "run build --from-bu1ld");
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

function writeProjectLocalPackageManager(root: string, manager: string): void {
  const bin = path.join(root, "node_modules", ".bin");
  mkdirSync(bin, { recursive: true });
  const runtime = path.join(bin, `${manager}-runtime.js`);
  writeRuntimeScript(runtime, manager);
  if (process.platform === "win32") {
    writeFileSync(path.join(bin, `${manager}.cmd`), [
      "@echo off",
      `node "%~dp0${manager}-runtime.js" %*`,
    ].join("\r\n"));
    return;
  }
  const shim = path.join(bin, manager);
  writeFileSync(shim, [
    "#!/bin/sh",
    `node "$(dirname "$0")/${manager}-runtime.js" "$@"`,
  ].join("\n"));
  chmodSync(shim, 0o755);
}

function writeRuntimeScript(file: string, manager: string): void {
  writeFileSync(file, [
    "const fs = require('node:fs');",
    "const path = require('node:path');",
    "const output = process.argv.slice(2).join(' ');",
    "fs.mkdirSync(path.join(process.cwd(), 'dist'), { recursive: true });",
    "fs.writeFileSync(path.join(process.cwd(), 'dist', 'out.txt'), output);",
    `console.log('${manager} runtime ' + output);`,
  ].join("\n"));
}

test("execute compiles TypeScript sources with the compiler API", async () => {
  const root = mkdtempSync(path.join(tmpdir(), "bu1ld-ts-plugin-exec-"));
  try {
    mkdirSync(path.join(root, "src"));
    writeFileSync(path.join(root, "src", "index.ts"), "export const answer: number = 42;\n");

    const result = await execute({
      namespace: "node",
      action: "compile",
      work_dir: root,
      params: {
        srcs: ["src/**/*.ts"],
        out_dir: "dist",
        module: "CommonJS",
        target: "ES2022",
      },
    });

    assert.match(result.output ?? "", /Compiled 1 TypeScript file/);
    const output = path.join(root, "dist", "index.js");
    assert.equal(existsSync(output), true);
    assert.match(readFileSync(output, "utf8"), /answer/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("execute typecheck reports compiler errors", async () => {
  const root = mkdtempSync(path.join(tmpdir(), "bu1ld-ts-plugin-error-"));
  try {
    mkdirSync(path.join(root, "src"));
    writeFileSync(path.join(root, "src", "index.ts"), "const value: string = 1;\n");

    await assert.rejects(async () => execute({
      namespace: "node",
      action: "typecheck",
      work_dir: root,
      params: {
        srcs: ["src/**/*.ts"],
      },
    }), /Type 'number' is not assignable to type 'string'/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
