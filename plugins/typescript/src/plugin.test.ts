import assert from "node:assert/strict";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync, mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";

import { configure, execute, expand, metadata } from "./plugin";

test("metadata exposes compiler-oriented TypeScript rules", () => {
  const result = metadata();
  assert.equal(result.id, "org.bu1ld.typescript");
  assert.equal(result.namespace, "typescript");
  assert.equal(result.auto_configure, true);
  assert.deepEqual(
    result.rules.map((rule) => rule.name),
    ["typecheck", "compile", "build"],
  );
});

test("build rule expands to a compiler plugin.exec task", () => {
  const [task] = expand({
    namespace: "typescript",
    rule: "build",
    target: "build",
    fields: {},
  });

  assert.equal(task.name, "build");
  assert.deepEqual(task.outputs, ["dist/**"]);
  assert.equal(task.action?.kind, "plugin.exec");
  assert.equal(task.action?.params.namespace, "typescript");
  assert.equal(task.action?.params.action, "compile");
});

test("configure creates compiler tasks without package.json", () => {
  const root = mkdtempSync(path.join(tmpdir(), "bu1ld-ts-plugin-"));
  const previous = process.env.BU1LD_PROJECT_DIR;
  process.env.BU1LD_PROJECT_DIR = root;
  try {
    mkdirSync(path.join(root, "src"));
    writeFileSync(path.join(root, "src", "index.ts"), "export const message: string = 'hello';\n");

    const tasks = configure({ namespace: "typescript", fields: {} });
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

test("execute compiles TypeScript sources with the compiler API", () => {
  const root = mkdtempSync(path.join(tmpdir(), "bu1ld-ts-plugin-exec-"));
  try {
    mkdirSync(path.join(root, "src"));
    writeFileSync(path.join(root, "src", "index.ts"), "export const answer: number = 42;\n");

    const result = execute({
      namespace: "typescript",
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

test("execute typecheck reports compiler errors", () => {
  const root = mkdtempSync(path.join(tmpdir(), "bu1ld-ts-plugin-error-"));
  try {
    mkdirSync(path.join(root, "src"));
    writeFileSync(path.join(root, "src", "index.ts"), "const value: string = 1;\n");

    assert.throws(() => execute({
      namespace: "typescript",
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
