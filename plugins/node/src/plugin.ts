import { spawnSync } from "node:child_process";
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

interface NpmPackageJson {
  content: Record<string, unknown>;
}

interface NpmRunScriptResult {
  stdout?: string | Buffer;
  stderr?: string | Buffer;
}

interface NpmRunScriptOptions {
  event: string;
  path: string;
  args?: string[];
  env?: Record<string, string>;
  pkg?: Record<string, unknown>;
  stdio?: "pipe";
  stdioString?: boolean;
}

const PackageJson = require("@npmcli/package-json") as {
  load(path: string): Promise<NpmPackageJson>;
};
const npmRunScript = require("@npmcli/run-script") as (options: NpmRunScriptOptions) => Promise<NpmRunScriptResult>;

const pluginID = "org.bu1ld.node";
const namespace = "node";

const defaultTsconfig = "tsconfig.json";
const defaultOutDir = "dist";
const defaultSources = ["src/**/*.ts", "src/**/*.tsx"];
const defaultPackageTaskPrefix = "node.";
const supportedPackageManagers = new Set(["npm", "pnpm", "yarn", "bun"]);
const packageManagerLockfiles: Record<string, string[]> = {
  bun: ["bun.lock", "bun.lockb"],
  pnpm: ["pnpm-lock.yaml"],
  yarn: ["yarn.lock"],
  npm: ["package-lock.json", "npm-shrinkwrap.json"],
};
const packageManagerConfigFiles: Record<string, string[]> = {
  bun: [],
  pnpm: ["pnpm-workspace.yaml", ".npmrc", ".pnpmfile.cjs"],
  yarn: [".yarnrc.yml", ".pnp.cjs"],
  npm: [".npmrc"],
};
const defaultInputs = [
  "build.bu1ld",
  "package.json",
  "package-lock.json",
  "npm-shrinkwrap.json",
  "pnpm-lock.yaml",
  "pnpm-workspace.yaml",
  "yarn.lock",
  ".yarnrc.yml",
  ".pnp.cjs",
  ".npmrc",
  ".pnpmfile.cjs",
  "bun.lock",
  "bun.lockb",
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
      rule("script", packageScriptFields()),
      rule("typecheck", taskFields()),
      rule("compile", taskFields()),
      rule("build", taskFields()),
    ],
    config_fields: [
      field("backend", "string"),
      field("import_scripts", "bool"),
      field("package_manager", "string"),
      field("scripts", "list"),
      field("task_prefix", "string"),
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

function packageScriptFields(): ReturnType<typeof field>[] {
  return [
    field("deps", "list"),
    field("inputs", "list"),
    field("outputs", "list"),
    field("script", "string"),
    field("package_manager", "string"),
    field("args", "list"),
  ];
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
    case "script":
      return [packageScriptTask(invocation.target, fields, projectDirectory())];
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
  const backend = resolveBackend(fields, projectDir);
  switch (backend) {
    case "package": {
      if (!optionalBool(fields, "import_scripts", true)) {
        return [];
      }
      const tasks = configurePackageScripts(projectDir, fields);
      if (tasks.length > 0) {
        return tasks;
      }
      if (!hasCompilerConfig(fields) && !looksLikeCompilerProject(projectDir)) {
        return [];
      }
      return configureCompiler(fields);
    }
    case "compiler":
      return configureCompiler(fields);
    case "none":
      return [];
    default:
      throw new Error(`node backend must be auto, package, compiler, or none`);
  }
}

function configureCompiler(fields: Record<string, unknown>): TaskSpec[] {
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

function configurePackageScripts(projectDir: string, fields: Record<string, unknown>): TaskSpec[] {
  const project = readPackageProject(projectDir);
  if (project === undefined) {
    return [];
  }
  const requested = optionalList(fields, "scripts", []);
  const scriptNames = requested.length > 0 ? requested : Object.keys(project.scripts);
  const prefix = optionalString(fields, "task_prefix", defaultPackageTaskPrefix);
  const manager = packageManager(projectDir, fields, project);
  return scriptNames
    .filter((script) => project.scripts[script] !== undefined)
    .map((script) => packageScriptTask(prefix + script, { ...fields, script, package_manager: manager }, projectDir));
}

function packageScriptTask(name: string, fields: Record<string, unknown>, projectDir: string): TaskSpec {
  const script = optionalString(fields, "script", name);
  const manager = packageManager(projectDir, fields, readPackageProject(projectDir));
  return {
    name,
    deps: optionalList(fields, "deps", []),
    inputs: optionalList(fields, "inputs", packageInputs(manager)),
    outputs: optionalList(fields, "outputs", defaultScriptOutputs(script)),
    action: action("script", {
      manager,
      script,
      args: optionalList(fields, "args", []),
    }),
  };
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
    outputs: optionalList(fields, "outputs", [outputPath(outDir)]),
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

export async function execute(request: ExecuteRequest): Promise<ExecuteResult> {
  switch (request.action) {
    case "script":
      return await runPackageScript(request.work_dir, request.params ?? {});
    case "typecheck":
      return runCompiler(request.work_dir, request.params ?? {}, true);
    case "compile":
      return runCompiler(request.work_dir, request.params ?? {}, false);
    default:
      throw new Error(`unknown node action "${request.action}"`);
  }
}

async function runPackageScript(workDir: string, params: Record<string, unknown>): Promise<ExecuteResult> {
  const manager = optionalString(params, "manager", "");
  const script = optionalString(params, "script", "");
  if (manager === "" || !supportedPackageManagers.has(manager)) {
    throw new Error(`node package manager must be one of ${Array.from(supportedPackageManagers).join(", ")}`);
  }
  if (script === "") {
    throw new Error("node script action requires script");
  }
  const args = optionalList(params, "args", []);
  if (manager === "npm") {
    return await runNpmPackageScript(workDir, script, args);
  }
  return runRuntimePackageScript(workDir, manager, script, args);
}

async function runNpmPackageScript(
  workDir: string,
  script: string,
  args: string[],
): Promise<ExecuteResult> {
  try {
    const pkg = await PackageJson.load(workDir);
    if (packageScripts(pkg.content)[script] === undefined) {
      throw new Error(`package.json does not define script "${script}"`);
    }
    const result = await npmRunScript({
      event: script,
      path: workDir,
      args,
      env: packageManagerScriptEnv("npm"),
      pkg: pkg.content,
      stdio: "pipe",
      stdioString: true,
    });
    const output = `${stringOutput(result.stdout)}${stringOutput(result.stderr)}`;
    return { output: output === "" ? `ran npm script ${script}\n` : output };
  } catch (error) {
    const output = scriptErrorOutput(error);
    const message = error instanceof Error ? error.message : String(error);
    throw new Error(`npm script "${script}" failed: ${message}${output === "" ? "" : `\n${output}`}`);
  }
}

function packageManagerScriptEnv(manager: string): Record<string, string> {
  return {
    npm_config_user_agent: `${manager}/bu1ld`,
    npm_node_execpath: process.execPath,
  };
}

function runRuntimePackageScript(workDir: string, manager: string, script: string, args: string[]): ExecuteResult {
  const project = readPackageProject(workDir);
  if (project === undefined) {
    throw new Error(`${manager} script "${script}" failed: package.json was not found`);
  }
  if (project.scripts[script] === undefined) {
    throw new Error(`${manager} script "${script}" failed: package.json does not define script "${script}"`);
  }
  let invocation: CommandInvocation;
  try {
    invocation = packageManagerInvocation(workDir, manager, project, script, args);
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    throw new Error(`${manager} script "${script}" failed: ${message}`);
  }
  return runCommandInvocation(workDir, manager, script, invocation);
}

function runCommandInvocation(workDir: string, manager: string, script: string, invocation: CommandInvocation): ExecuteResult {
  const result = spawnSync(invocation.command, invocation.args, {
    cwd: workDir,
    encoding: "utf8",
    env: {
      ...process.env,
      COREPACK_ENABLE_DOWNLOAD_PROMPT: "0",
    },
  });
  const output = `${result.stdout ?? ""}${result.stderr ?? ""}`;
  if (result.error !== undefined) {
    throw new Error(`run ${manager} script "${script}": ${result.error.message}`);
  }
  if (result.status !== 0) {
    throw new Error(`${manager} script "${script}" exited with code ${result.status}\n${output.trim()}`);
  }
  return { output: output === "" ? `ran ${manager} script ${script}\n` : output };
}

function stringOutput(value: string | Buffer | undefined): string {
  if (value === undefined) {
    return "";
  }
  return typeof value === "string" ? value : value.toString("utf8");
}

function scriptErrorOutput(error: unknown): string {
  if (typeof error !== "object" || error === null) {
    return "";
  }
  const values = error as { stdout?: string | Buffer; stderr?: string | Buffer };
  return `${stringOutput(values.stdout)}${stringOutput(values.stderr)}`.trim();
}

interface CommandInvocation {
  command: string;
  args: string[];
}

function packageManagerInvocation(
  projectDir: string,
  manager: string,
  project: PackageProject,
  script: string,
  args: string[],
): CommandInvocation {
  if (manager === "yarn") {
    const yarnPath = yarnPathFromConfig(projectDir);
    if (yarnPath !== undefined) {
      return nodeScriptInvocation(yarnPath, packageManagerRunArgs(manager, script, args));
    }
  }

  const local = localPackageManagerBin(projectDir, manager);
  if (local !== undefined) {
    return executableInvocation(local, packageManagerRunArgs(manager, script, args));
  }

  const declared = parsePackageManagerDeclaration(project.packageManager);
  if (declared?.name === manager && declared.version !== undefined) {
    return corepackInvocation(manager, declared.version, script, args);
  }

  if (manager === "bun") {
    return executableInvocation("bun", packageManagerRunArgs(manager, script, args));
  }

  return executableInvocation(manager, packageManagerRunArgs(manager, script, args));
}

function corepackInvocation(manager: string, version: string, script: string, args: string[]): CommandInvocation {
  return executableInvocation("corepack", [`${manager}@${version}`, ...packageManagerRunArgs(manager, script, args)]);
}

function nodeScriptInvocation(scriptPath: string, args: string[]): CommandInvocation {
  return {
    command: process.execPath,
    args: [scriptPath, ...args],
  };
}

function executableInvocation(executable: string, args: string[]): CommandInvocation {
  if (process.platform !== "win32" || executable === process.execPath) {
    return { command: executable, args };
  }
  const commandParts = windowsCommandParts(executable, args);
  return {
    command: process.env.ComSpec ?? "cmd.exe",
    args: ["/d", "/s", "/c", quoteWindowsCommand(commandParts)],
  };
}

function windowsCommandParts(executable: string, args: string[]): string[] {
  const command = windowsExecutable(executable);
  const ext = path.extname(command).toLowerCase();
  if (ext === ".cmd" || ext === ".bat") {
    return ["call", command, ...args];
  }
  return [command, ...args];
}

function windowsExecutable(executable: string): string {
  if (path.isAbsolute(executable) || path.extname(executable) !== "") {
    return executable;
  }
  return executable === "bun" ? executable : `${executable}.cmd`;
}

function localPackageManagerBin(projectDir: string, manager: string): string | undefined {
  const binDir = path.join(projectDir, "node_modules", ".bin");
  const candidates = process.platform === "win32"
    ? [`${manager}.cmd`, `${manager}.exe`, manager]
    : [manager];
  for (const candidate of candidates) {
    const file = path.join(binDir, candidate);
    if (existsSync(file)) {
      return file;
    }
  }
  return undefined;
}

function yarnPathFromConfig(projectDir: string): string | undefined {
  const config = path.join(projectDir, ".yarnrc.yml");
  if (!existsSync(config)) {
    return undefined;
  }
  const value = readYarnPath(readFileSync(config, "utf8"));
  if (value === undefined) {
    return undefined;
  }
  const resolved = path.resolve(projectDir, value);
  if (!existsSync(resolved)) {
    throw new Error(`yarnPath "${value}" was not found`);
  }
  return resolved;
}

function readYarnPath(contents: string): string | undefined {
  for (const line of contents.split(/\r?\n/)) {
    const match = line.match(/^\s*yarnPath\s*:\s*(.+?)\s*$/);
    if (match === null) {
      continue;
    }
    return unquoteYamlScalar(match[1].trim());
  }
  return undefined;
}

function unquoteYamlScalar(value: string): string {
  if (value.length >= 2) {
    const first = value[0];
    const last = value[value.length - 1];
    if ((first === `"` && last === `"`) || (first === `'` && last === `'`)) {
      return value.slice(1, -1);
    }
  }
  return value;
}

function packageManagerRunArgs(manager: string, script: string, args: string[]): string[] {
  if (manager === "npm") {
    return args.length > 0 ? ["run", script, "--", ...args] : ["run", script];
  }
  return ["run", script, ...args];
}

function quoteWindowsCommand(parts: string[]): string {
  return parts.map(quoteWindowsArg).join(" ");
}

function quoteWindowsArg(value: string): string {
  if (/^[A-Za-z0-9_./:=+\\-]+$/.test(value)) {
    return value;
  }
  return `"${value.replaceAll('"', '\\"')}"`;
}

function runCompiler(workDir: string, params: Record<string, unknown>, noEmit: boolean): ExecuteResult {
  const project = loadCompilerProject(workDir, params, noEmit);
  if (project.fileNames.length === 0) {
    throw new Error("TypeScript compiler found no source files");
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

function resolveBackend(fields: Record<string, unknown>, projectDir: string): string {
  const requested = optionalString(fields, "backend", "auto").trim().toLowerCase();
  if (requested !== "auto") {
    return requested;
  }
  const project = readPackageProject(projectDir);
  if (project !== undefined && Object.keys(project.scripts).length > 0) {
    return "package";
  }
  if (hasCompilerConfig(fields) || looksLikeCompilerProject(projectDir)) {
    return "compiler";
  }
  if (project !== undefined) {
    return "package";
  }
  return "none";
}

interface PackageProject {
  packageManager?: string;
  scripts: Record<string, string>;
}

interface PackageManagerDeclaration {
  name: string;
  version?: string;
}

function readPackageProject(projectDir: string): PackageProject | undefined {
  const file = path.join(projectDir, "package.json");
  if (!existsSync(file)) {
    return undefined;
  }
  const parsed = JSON.parse(readFileSync(file, "utf8")) as Record<string, unknown>;
  const scripts = parsed.scripts;
  const packageManagerValue = parsed.packageManager;
  return {
    packageManager: typeof packageManagerValue === "string" ? packageManagerValue : undefined,
    scripts: packageScripts(parsed),
  };
}

function packageScripts(pkg: Record<string, unknown>): Record<string, string> {
  const scripts = pkg.scripts;
  return typeof scripts === "object" && scripts !== null && !Array.isArray(scripts)
    ? stringRecord(scripts)
    : {};
}

function stringRecord(value: object): Record<string, string> {
  const result: Record<string, string> = {};
  for (const [key, item] of Object.entries(value)) {
    if (typeof item === "string") {
      result[key] = item;
    }
  }
  return result;
}

function packageManager(projectDir: string, fields: Record<string, unknown>, project?: PackageProject): string {
  const configured = optionalString(fields, "package_manager", "");
  if (configured !== "") {
    return validatePackageManager(configured);
  }
  const declared = parsePackageManagerDeclaration(project?.packageManager)?.name;
  if (declared !== undefined) {
    return validatePackageManager(declared);
  }
  for (const [manager, lockfiles] of Object.entries(packageManagerLockfiles)) {
    if (lockfiles.some((lockfile) => existsSync(path.join(projectDir, lockfile)))) {
      return manager;
    }
  }
  return "npm";
}

function parsePackageManagerDeclaration(value: string | undefined): PackageManagerDeclaration | undefined {
  const trimmed = value?.trim();
  if (trimmed === undefined || trimmed === "") {
    return undefined;
  }
  const separator = trimmed.indexOf("@", 1);
  if (separator === -1) {
    return {
      name: trimmed.toLowerCase(),
    };
  }
  const name = trimmed.slice(0, separator).toLowerCase();
  const version = trimmed.slice(separator + 1).trim();
  return version === "" ? { name } : { name, version };
}

function validatePackageManager(manager: string): string {
  const normalized = manager.trim().toLowerCase();
  if (!supportedPackageManagers.has(normalized)) {
    throw new Error(`node package_manager must be one of ${Array.from(supportedPackageManagers).join(", ")}`);
  }
  return normalized;
}

function packageInputs(manager: string): string[] {
  return [
    "build.bu1ld",
    "package.json",
    ...packageManagerLockfiles[manager],
    ...packageManagerConfigFiles[manager],
    defaultTsconfig,
    "src/**",
  ];
}

function defaultScriptOutputs(script: string): string[] {
  return script === "build" ? [outputPath(defaultOutDir)] : [];
}

function hasCompilerConfig(fields: Record<string, unknown>): boolean {
  return [
    "typecheck",
    "build",
    "srcs",
    "inputs",
    "out_dir",
    "tsconfig",
    "root_dir",
    "target",
    "module",
    "module_resolution",
    "jsx",
    "strict",
    "declaration",
    "source_map",
    "incremental",
    "no_emit_on_error",
    "skip_lib_check",
    "allow_js",
    "check_js",
    "lib",
    "types",
    "type_roots",
    "base_url",
    "paths",
  ].some((fieldName) => hasField(fields, fieldName));
}

function looksLikeCompilerProject(projectDir: string): boolean {
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

function outputPath(out: string): string {
  const trimmed = out.replace(/[\\/]+$/, "");
  return trimmed === "" ? "." : trimmed;
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
    throw new Error(`node field "${name}" must be string`);
  }
  return value;
}

function optionalBool(fields: Record<string, unknown>, name: string, fallback: boolean): boolean {
  const value = fields[name];
  if (value === undefined || value === null) {
    return fallback;
  }
  if (typeof value !== "boolean") {
    throw new Error(`node field "${name}" must be bool`);
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
    throw new Error(`node field "${name}" must be list`);
  }
  return value;
}

function optionalObject(fields: Record<string, unknown>, name: string, fallback: Record<string, unknown>): Record<string, unknown> {
  const value = fields[name];
  if (value === undefined || value === null) {
    return fallback;
  }
  if (typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`node field "${name}" must be object`);
  }
  return value as Record<string, unknown>;
}

function uniqueSorted(values: string[]): string[] {
  return Array.from(new Set(values)).sort();
}

function toSlash(value: string): string {
  return value.replaceAll(path.sep, "/");
}
