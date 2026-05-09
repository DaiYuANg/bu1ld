import { existsSync, readdirSync, readFileSync } from "node:fs";
import path from "node:path";

import ts from "typescript";

import {
  ExecuteRequest,
  ExecuteResult,
  Invocation,
  Metadata,
  PluginConfig,
  TaskAction,
  TaskSpec,
  field,
  pluginExecActionKind,
  protocolVersion,
  rule,
} from "./protocol";

const pluginID = "org.bu1ld.typescript";
const namespace = "typescript";

const defaultTsconfig = "tsconfig.json";
const defaultOutDir = "dist";
const defaultSources = ["src/**/*.ts", "src/**/*.tsx"];
const defaultInputs = [
  "build.bu1ld",
  defaultTsconfig,
  "src/**/*.ts",
  "src/**/*.tsx",
  "src/**/*.d.ts",
];
const ignoredDirectories = new Set([".bu1ld", ".git", "dist", "build", "node_modules"]);

export function metadata(): Metadata {
  return {
    id: pluginID,
    namespace,
    protocol_version: protocolVersion,
    capabilities: ["metadata", "expand", "configure", "execute"],
    rules: [
      rule("typecheck", taskFields()),
      rule("compile", taskFields()),
      rule("build", taskFields()),
    ],
    config_fields: [
      field("typecheck", "bool"),
      field("build", "bool"),
      field("srcs", "list"),
      field("inputs", "list"),
      field("out_dir", "string"),
      field("tsconfig", "string"),
      field("root_dir", "string"),
      field("target", "string"),
      field("module", "string"),
      field("module_resolution", "string"),
      field("jsx", "string"),
      field("strict", "bool"),
      field("declaration", "bool"),
      field("source_map", "bool"),
      field("incremental", "bool"),
      field("no_emit_on_error", "bool"),
      field("skip_lib_check", "bool"),
      field("allow_js", "bool"),
      field("check_js", "bool"),
      field("lib", "list"),
      field("types", "list"),
      field("type_roots", "list"),
      field("base_url", "string"),
      field("paths", "object"),
    ],
    auto_configure: true,
  };
}

function taskFields(): ReturnType<typeof field>[] {
  return [
    field("deps", "list"),
    field("inputs", "list"),
    field("outputs", "list"),
    field("srcs", "list"),
    field("out_dir", "string"),
    field("tsconfig", "string"),
    field("root_dir", "string"),
    field("target", "string"),
    field("module", "string"),
    field("module_resolution", "string"),
    field("jsx", "string"),
    field("strict", "bool"),
    field("declaration", "bool"),
    field("source_map", "bool"),
    field("incremental", "bool"),
    field("no_emit_on_error", "bool"),
    field("skip_lib_check", "bool"),
    field("allow_js", "bool"),
    field("check_js", "bool"),
    field("lib", "list"),
    field("types", "list"),
    field("type_roots", "list"),
    field("base_url", "string"),
    field("paths", "object"),
  ];
}

export function expand(invocation: Invocation): TaskSpec[] {
  const fields = fieldsOf(invocation);
  switch (invocation.rule) {
    case "typecheck":
      return [typecheckTask(invocation.target, fields)];
    case "compile":
    case "build":
      return [compileTask(invocation.target, fields)];
    default:
      return [];
  }
}

export function configure(config: PluginConfig): TaskSpec[] {
  const fields = config.fields ?? {};
  const projectDir = projectDirectory();
  const explicitConfig = Object.keys(fields).length > 0;
  if (!explicitConfig && !looksLikeTypeScriptProject(projectDir)) {
    return [];
  }

  const tasks: TaskSpec[] = [];
  const typecheckEnabled = optionalBool(fields, "typecheck", true);
  const buildEnabled = optionalBool(fields, "build", true);
  if (typecheckEnabled) {
    tasks.push(typecheckTask("typecheck", fields));
  }
  if (buildEnabled) {
    tasks.push(compileTask("build", withDeps(fields, typecheckEnabled ? ["typecheck"] : [])));
  }
  return tasks;
}

function typecheckTask(name: string, fields: Record<string, unknown>): TaskSpec {
  return {
    name,
    deps: optionalList(fields, "deps", []),
    inputs: optionalList(fields, "inputs", defaultInputs),
    action: action("typecheck", compilerParams(fields)),
  };
}

function compileTask(name: string, fields: Record<string, unknown>): TaskSpec {
  const outDir = optionalString(fields, "out_dir", defaultOutDir);
  return {
    name,
    deps: optionalList(fields, "deps", []),
    inputs: optionalList(fields, "inputs", defaultInputs),
    outputs: optionalList(fields, "outputs", [outputPattern(outDir)]),
    action: action("compile", compilerParams(fields)),
  };
}

function compilerParams(fields: Record<string, unknown>): Record<string, unknown> {
  const params: Record<string, unknown> = {};
  copyField(fields, params, "srcs");
  copyField(fields, params, "out_dir");
  copyField(fields, params, "tsconfig");
  copyField(fields, params, "root_dir");
  copyField(fields, params, "target");
  copyField(fields, params, "module");
  copyField(fields, params, "module_resolution");
  copyField(fields, params, "jsx");
  copyField(fields, params, "strict");
  copyField(fields, params, "declaration");
  copyField(fields, params, "source_map");
  copyField(fields, params, "incremental");
  copyField(fields, params, "no_emit_on_error");
  copyField(fields, params, "skip_lib_check");
  copyField(fields, params, "allow_js");
  copyField(fields, params, "check_js");
  copyField(fields, params, "lib");
  copyField(fields, params, "types");
  copyField(fields, params, "type_roots");
  copyField(fields, params, "base_url");
  copyField(fields, params, "paths");
  return params;
}

function action(actionName: string, params: Record<string, unknown>): TaskAction {
  return {
    kind: pluginExecActionKind,
    params: {
      namespace,
      action: actionName,
      params,
    },
  };
}

export function execute(request: ExecuteRequest): ExecuteResult {
  switch (request.action) {
    case "typecheck":
      return runCompiler(request.work_dir, request.params ?? {}, true);
    case "compile":
      return runCompiler(request.work_dir, request.params ?? {}, false);
    default:
      throw new Error(`unknown typescript action "${request.action}"`);
  }
}

function runCompiler(workDir: string, params: Record<string, unknown>, noEmit: boolean): ExecuteResult {
  const project = loadCompilerProject(workDir, params, noEmit);
  if (project.fileNames.length === 0) {
    throw new Error("typescript compiler found no source files");
  }

  const program = ts.createProgram(project.fileNames, project.options);
  const emit = noEmit ? undefined : program.emit();
  const diagnostics = [
    ...ts.getPreEmitDiagnostics(program),
    ...(emit?.diagnostics ?? []),
  ];
  const errors = diagnostics.filter((diagnostic) => diagnostic.category === ts.DiagnosticCategory.Error);
  if (errors.length > 0 || emit?.emitSkipped) {
    throw new Error(formatDiagnostics(diagnostics, workDir));
  }

  const warnings = diagnostics.filter((diagnostic) => diagnostic.category !== ts.DiagnosticCategory.Error);
  const summary = noEmit
    ? `Type checked ${project.fileNames.length} TypeScript file(s).\n`
    : `Compiled ${project.fileNames.length} TypeScript file(s) to ${project.relativeOutDir}.\n`;
  const warningText = warnings.length > 0 ? `${formatDiagnostics(warnings, workDir)}\n` : "";
  return { output: warningText + summary };
}

interface CompilerProject {
  fileNames: string[];
  options: ts.CompilerOptions;
  relativeOutDir: string;
}

function loadCompilerProject(workDir: string, params: Record<string, unknown>, noEmit: boolean): CompilerProject {
  const root = path.resolve(workDir);
  const tsconfig = optionalString(params, "tsconfig", defaultTsconfig);
  const configPath = path.resolve(root, tsconfig);
  const hasConfig = existsSync(configPath);
  let fileNames: string[] = [];
  let options: ts.CompilerOptions = {};

  if (hasConfig) {
    const config = readTsconfig(configPath);
    options = config.options;
    fileNames = config.fileNames;
  } else if (params.tsconfig !== undefined && tsconfig !== defaultTsconfig) {
    throw new Error(`tsconfig "${tsconfig}" was not found`);
  }

  options = {
    ...options,
    ...compilerOptionsFromParams(root, params, hasConfig),
    noEmit,
  };

  const srcs = optionalList(params, "srcs", []);
  if (srcs.length > 0) {
    fileNames = expandSourcePatterns(root, srcs, options.allowJs === true);
  } else if (!hasConfig) {
    fileNames = expandSourcePatterns(root, defaultSources, false);
  }

  const relativeOutDir = optionalString(params, "out_dir", defaultOutDir);
  if (!noEmit && options.outDir === undefined) {
    options.outDir = path.resolve(root, relativeOutDir);
  }

  return {
    fileNames: uniqueSorted(fileNames.map((file) => path.resolve(file))),
    options,
    relativeOutDir,
  };
}

function compilerOptionsFromParams(root: string, params: Record<string, unknown>, hasConfig: boolean): ts.CompilerOptions {
  const json: Record<string, unknown> = hasConfig ? {} : {
    target: "ES2022",
    module: "CommonJS",
    moduleResolution: "Node",
    strict: true,
    noEmitOnError: true,
    skipLibCheck: true,
  };

  setStringOption(json, "rootDir", params, "root_dir");
  setStringOption(json, "outDir", params, "out_dir");
  setStringOption(json, "target", params, "target");
  setStringOption(json, "module", params, "module");
  setStringOption(json, "moduleResolution", params, "module_resolution");
  setStringOption(json, "jsx", params, "jsx");
  setStringOption(json, "baseUrl", params, "base_url");
  setBoolOption(json, "strict", params, "strict");
  setBoolOption(json, "declaration", params, "declaration");
  setBoolOption(json, "sourceMap", params, "source_map");
  setBoolOption(json, "incremental", params, "incremental");
  setBoolOption(json, "noEmitOnError", params, "no_emit_on_error");
  setBoolOption(json, "skipLibCheck", params, "skip_lib_check");
  setBoolOption(json, "allowJs", params, "allow_js");
  setBoolOption(json, "checkJs", params, "check_js");
  setListOption(json, "lib", params, "lib");
  setListOption(json, "types", params, "types");
  setListOption(json, "typeRoots", params, "type_roots");
  const paths = optionalObject(params, "paths", {});
  if (Object.keys(paths).length > 0) {
    json.paths = paths;
  }

  const converted = ts.convertCompilerOptionsFromJson(json, root);
  if (converted.errors.length > 0) {
    throw new Error(formatDiagnostics(converted.errors, root));
  }
  return converted.options;
}

function setStringOption(json: Record<string, unknown>, option: string, params: Record<string, unknown>, fieldName: string): void {
  if (!hasField(params, fieldName)) {
    return;
  }
  const value = optionalString(params, fieldName, "");
  if (value !== "") {
    json[option] = value;
  }
}

function setBoolOption(json: Record<string, unknown>, option: string, params: Record<string, unknown>, fieldName: string): void {
  if (hasField(params, fieldName)) {
    json[option] = optionalBool(params, fieldName, false);
  }
}

function setListOption(json: Record<string, unknown>, option: string, params: Record<string, unknown>, fieldName: string): void {
  if (!hasField(params, fieldName)) {
    return;
  }
  const value = optionalList(params, fieldName, []);
  if (value.length > 0) {
    json[option] = value;
  }
}

function readTsconfig(configPath: string): ts.ParsedCommandLine {
  const loaded = ts.readConfigFile(configPath, (file) => readFileSync(file, "utf8"));
  if (loaded.error) {
    throw new Error(formatDiagnostics([loaded.error], path.dirname(configPath)));
  }
  const parsed = ts.parseJsonConfigFileContent(
    loaded.config,
    ts.sys,
    path.dirname(configPath),
    undefined,
    configPath,
  );
  if (parsed.errors.length > 0) {
    throw new Error(formatDiagnostics(parsed.errors, path.dirname(configPath)));
  }
  return parsed;
}

function expandSourcePatterns(root: string, patterns: string[], allowJs: boolean): string[] {
  const files = listProjectFiles(root, allowJs);
  const matches = files.filter((file) => {
    const relative = toSlash(path.relative(root, file));
    return patterns.some((pattern) => globToRegExp(toSlash(pattern)).test(relative));
  });
  return uniqueSorted(matches);
}

function listProjectFiles(root: string, allowJs: boolean): string[] {
  const files: string[] = [];
  walk(root, files, allowJs);
  return files;
}

function walk(dir: string, files: string[], allowJs: boolean): void {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (entry.isDirectory()) {
      if (!ignoredDirectories.has(entry.name)) {
        walk(path.join(dir, entry.name), files, allowJs);
      }
      continue;
    }
    if (!entry.isFile()) {
      continue;
    }
    const ext = path.extname(entry.name);
    if (ext === ".ts" || ext === ".tsx" || (allowJs && (ext === ".js" || ext === ".jsx"))) {
      files.push(path.join(dir, entry.name));
    }
  }
}

function globToRegExp(pattern: string): RegExp {
  let regex = "^";
  for (let i = 0; i < pattern.length; i++) {
    const char = pattern[i];
    const next = pattern[i + 1];
    if (char === "*" && next === "*") {
      const after = pattern[i + 2];
      if (after === "/") {
        regex += "(?:.*/)?";
        i += 2;
      } else {
        regex += ".*";
        i++;
      }
      continue;
    }
    if (char === "*") {
      regex += "[^/]*";
      continue;
    }
    regex += escapeRegExp(char);
  }
  regex += "$";
  return new RegExp(regex);
}

function escapeRegExp(char: string): string {
  return /[\\^$+?.()|{}[\]]/.test(char) ? `\\${char}` : char;
}

function formatDiagnostics(diagnostics: readonly ts.Diagnostic[], root: string): string {
  const host: ts.FormatDiagnosticsHost = {
    getCanonicalFileName: (file) => file,
    getCurrentDirectory: () => root,
    getNewLine: () => "\n",
  };
  return ts.formatDiagnosticsWithColorAndContext(diagnostics, host);
}

function looksLikeTypeScriptProject(projectDir: string): boolean {
  if (existsSync(path.join(projectDir, defaultTsconfig))) {
    return true;
  }
  return expandSourcePatterns(projectDir, defaultSources, false).length > 0;
}

function fieldsOf(invocation: Invocation): Record<string, unknown> {
  return invocation.fields ?? {};
}

function copyField(source: Record<string, unknown>, target: Record<string, unknown>, name: string): void {
  if (hasField(source, name)) {
    target[name] = source[name];
  }
}

function hasField(values: Record<string, unknown>, name: string): boolean {
  return Object.prototype.hasOwnProperty.call(values, name);
}

function withDeps(fields: Record<string, unknown>, deps: string[]): Record<string, unknown> {
  return { ...fields, deps };
}

function outputPattern(out: string): string {
  const trimmed = out.replace(/[\\/]+$/, "");
  return trimmed === "" ? "**" : `${trimmed}/**`;
}

function projectDirectory(): string {
  return process.env.BU1LD_PROJECT_DIR || process.cwd();
}

function optionalString(fields: Record<string, unknown>, name: string, fallback: string): string {
  const value = fields[name];
  if (value === undefined || value === null) {
    return fallback;
  }
  if (typeof value !== "string") {
    throw new Error(`typescript field "${name}" must be string`);
  }
  return value;
}

function optionalBool(fields: Record<string, unknown>, name: string, fallback: boolean): boolean {
  const value = fields[name];
  if (value === undefined || value === null) {
    return fallback;
  }
  if (typeof value !== "boolean") {
    throw new Error(`typescript field "${name}" must be bool`);
  }
  return value;
}

function optionalList(fields: Record<string, unknown>, name: string, fallback: string[]): string[] {
  const value = fields[name];
  if (value === undefined || value === null) {
    return fallback;
  }
  if (typeof value === "string") {
    return [value];
  }
  if (!Array.isArray(value) || value.some((item) => typeof item !== "string")) {
    throw new Error(`typescript field "${name}" must be list`);
  }
  return value;
}

function optionalObject(fields: Record<string, unknown>, name: string, fallback: Record<string, unknown>): Record<string, unknown> {
  const value = fields[name];
  if (value === undefined || value === null) {
    return fallback;
  }
  if (typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`typescript field "${name}" must be object`);
  }
  return value as Record<string, unknown>;
}

function uniqueSorted(values: string[]): string[] {
  return Array.from(new Set(values)).sort();
}

function toSlash(value: string): string {
  return value.replaceAll(path.sep, "/");
}
