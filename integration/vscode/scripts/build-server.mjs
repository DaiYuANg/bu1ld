import { spawnSync } from 'node:child_process';
import { chmodSync, mkdirSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const extensionDir = resolve(scriptDir, '..');
const repoRoot = resolve(extensionDir, '..', '..');
const buildAll = process.argv.includes('--all');

const targets = [
  { vscode: 'darwin-arm64', goos: 'darwin', goarch: 'arm64', exe: 'bu1ld-lsp' },
  { vscode: 'darwin-x64', goos: 'darwin', goarch: 'amd64', exe: 'bu1ld-lsp' },
  { vscode: 'linux-arm64', goos: 'linux', goarch: 'arm64', exe: 'bu1ld-lsp' },
  { vscode: 'linux-x64', goos: 'linux', goarch: 'amd64', exe: 'bu1ld-lsp' },
  { vscode: 'win32-arm64', goos: 'windows', goarch: 'arm64', exe: 'bu1ld-lsp.exe' },
  { vscode: 'win32-x64', goos: 'windows', goarch: 'amd64', exe: 'bu1ld-lsp.exe' },
];

const selectedTargets = buildAll ? targets : [currentTarget()];

for (const target of selectedTargets) {
  buildTarget(target);
}

function currentTarget() {
  const target = targets.find((item) => item.vscode === `${process.platform}-${process.arch}`);
  if (target === undefined) {
    throw new Error(`unsupported platform ${process.platform}-${process.arch}`);
  }
  return target;
}

function buildTarget(target) {
  const out = resolve(extensionDir, 'server', target.vscode, target.exe);
  mkdirSync(dirname(out), { recursive: true });

  const result = spawnSync('go', ['build', '-trimpath', '-o', out, './cmd/lsp'], {
    cwd: repoRoot,
    env: {
      ...process.env,
      CGO_ENABLED: '0',
      GOOS: target.goos,
      GOARCH: target.goarch,
    },
    stdio: 'inherit',
  });

  if (result.error !== undefined) {
    throw result.error;
  }
  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
  if (target.goos !== 'windows') {
    chmodSync(out, 0o755);
  }
}
