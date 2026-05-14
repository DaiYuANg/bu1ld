export const protocolVersion = 1;
export const pluginExecActionKind = "plugin.exec";

export type FieldType = "string" | "list" | "object" | "bool";

export interface Metadata {
  id: string;
  namespace: string;
  protocol_version: number;
  capabilities: string[];
  rules: RuleSchema[];
  config_fields?: FieldSchema[];
  auto_configure?: boolean;
}

export interface MetadataResult {
  metadata: Metadata;
}

export interface FieldSchema {
  name: string;
  type: FieldType;
  required?: boolean;
}

export interface RuleSchema {
  name: string;
  fields: FieldSchema[];
}

export interface Invocation {
  namespace: string;
  rule: string;
  target: string;
  fields?: Record<string, unknown>;
}

export interface ExpandParams {
  invocation: Invocation;
}

export interface ExpandResult {
  tasks: TaskSpec[];
}

export interface PluginConfig {
  namespace: string;
  fields?: Record<string, unknown>;
}

export interface ConfigureParams {
  config: PluginConfig;
}

export interface ConfigureResult {
  tasks: TaskSpec[];
}

export interface ExecuteRequest {
  namespace: string;
  action: string;
  work_dir: string;
  params?: Record<string, unknown>;
}

export interface ExecuteParams {
  request: ExecuteRequest;
}

export interface ExecuteResult {
  output?: string;
}

export interface TaskSpec {
  name: string;
  deps?: string[];
  inputs?: string[];
  outputs?: string[];
  command?: string[];
  action?: TaskAction;
}

export interface TaskAction {
  kind: string;
  params: Record<string, unknown>;
}

export function field(name: string, type: FieldType, required = false): FieldSchema {
  return { name, type, required: required || undefined };
}

export function rule(name: string, fields: FieldSchema[]): RuleSchema {
  return { name, fields };
}
