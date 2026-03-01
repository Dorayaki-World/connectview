import * as vscode from "vscode";
import * as path from "path";

export interface Config {
  protocPath: string;
  pluginPath: string;
  protoRoot: string;
  workspaceRoot: string;
  includePaths: string[];
}

export function loadConfig(): Config {
  const ws = vscode.workspace.workspaceFolders?.[0];
  const root = ws?.uri.fsPath ?? "";
  const cfg = vscode.workspace.getConfiguration("connectview");

  const resolve = (p: string): string => {
    if (!p || path.isAbsolute(p)) return p;
    return path.resolve(root, p);
  };

  const protoRootRaw = cfg.get<string>("protoRoot", "");
  const protoRoot = protoRootRaw ? resolve(protoRootRaw) : root;

  const includePaths = cfg
    .get<string[]>("includePaths", [])
    .map(resolve);

  return {
    protocPath: cfg.get<string>("protocPath", "protoc"),
    pluginPath: cfg.get<string>("pluginPath", "protoc-gen-connectview"),
    protoRoot,
    workspaceRoot: root,
    includePaths,
  };
}
