import * as vscode from 'vscode';
import * as fs from 'node:fs';
import * as path from 'node:path';
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
  State,
  Trace,
} from 'vscode-languageclient/node';

let client: LanguageClient | undefined;

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  context.subscriptions.push(
    vscode.commands.registerCommand('bu1ld.restartLsp', async () => {
      await restartClient(context);
      void vscode.window.showInformationMessage('bu1ld LSP restarted');
    }),
  );

  await startClient(context);
}

export async function deactivate(): Promise<void> {
  await stopClient();
}

async function restartClient(context: vscode.ExtensionContext): Promise<void> {
  await stopClient();
  await startClient(context);
}

async function startClient(context: vscode.ExtensionContext): Promise<void> {
  if (client !== undefined && client.state !== State.Stopped) {
    return;
  }

  const outputChannel = vscode.window.createOutputChannel('bu1ld LSP');
  context.subscriptions.push(outputChannel);

  client = new LanguageClient(
    'bu1ld',
    'bu1ld Language Server',
    serverOptions(context),
    clientOptions(outputChannel),
  );
  client.setTrace(traceSetting());

  context.subscriptions.push(client);
  await client.start();
}

async function stopClient(): Promise<void> {
  if (client === undefined) {
    return;
  }

  const current = client;
  client = undefined;
  await current.stop();
}

function serverOptions(context: vscode.ExtensionContext): ServerOptions {
  const config = vscode.workspace.getConfiguration('bu1ld.lsp');
  const command = lspCommand(context, config);
  const args = config.get<string[]>('args') ?? ['stdio'];
  const cwd = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;

  return {
    run: { command, args, options: { cwd } },
    debug: { command, args, options: { cwd } },
  };
}

function lspCommand(context: vscode.ExtensionContext, config: vscode.WorkspaceConfiguration): string {
  const configured = config.get<string>('path')?.trim();
  if (configured !== undefined && configured !== '' && isUserConfigured(config, 'path')) {
    return configured;
  }

  const bundled = bundledServerPath(context);
  if (bundled !== undefined) {
    return bundled;
  }

  return configured || 'bu1ld-lsp';
}

function isUserConfigured(config: vscode.WorkspaceConfiguration, key: string): boolean {
  const inspected = config.inspect<string>(key);
  return inspected?.globalValue !== undefined
    || inspected?.workspaceValue !== undefined
    || inspected?.workspaceFolderValue !== undefined;
}

function bundledServerPath(context: vscode.ExtensionContext): string | undefined {
  const executable = process.platform === 'win32' ? 'bu1ld-lsp.exe' : 'bu1ld-lsp';
  const candidate = path.join(context.extensionPath, 'server', serverPlatform(), executable);
  if (!fs.existsSync(candidate)) {
    return undefined;
  }
  return candidate;
}

function serverPlatform(): string {
  return `${process.platform}-${process.arch}`;
}

function clientOptions(outputChannel: vscode.OutputChannel): LanguageClientOptions {
  return {
    documentSelector: [
      { scheme: 'file', language: 'bu1ld' },
      { scheme: 'untitled', language: 'bu1ld' },
    ],
    synchronize: {
      fileEvents: vscode.workspace.createFileSystemWatcher('**/*.bu1ld'),
      configurationSection: 'bu1ld.lsp',
    },
    outputChannel,
  };
}

function traceSetting(): Trace {
  const value = vscode.workspace.getConfiguration('bu1ld.lsp').get<string>('trace.server') ?? 'off';
  switch (value) {
    case 'messages':
      return Trace.Messages;
    case 'verbose':
      return Trace.Verbose;
    default:
      return Trace.Off;
  }
}
