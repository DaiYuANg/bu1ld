#!/usr/bin/env node

import {
  StreamMessageReader,
  StreamMessageWriter,
  createMessageConnection,
} from "vscode-jsonrpc/node";

import {
  ConfigureParams,
  ExecuteParams,
  ExpandParams,
  MetadataResult,
} from "./protocol";
import { configure, execute, expand, metadata } from "./plugin";

const connection = createMessageConnection(
  new StreamMessageReader(process.stdin),
  new StreamMessageWriter(process.stdout),
);

connection.onRequest("metadata", (): MetadataResult => ({ metadata: metadata() }));
connection.onRequest("expand", (params: ExpandParams) => ({ tasks: expand(params.invocation) }));
connection.onRequest("configure", (params: ConfigureParams) => ({ tasks: configure(params.config) }));
connection.onRequest("execute", async (params: ExecuteParams) => execute(params.request));

connection.onError((error) => {
  process.stderr.write(`bu1ld-typescript-plugin JSON-RPC error: ${String(error)}\n`);
});

connection.listen();
