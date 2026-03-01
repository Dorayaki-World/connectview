import * as vscode from "vscode";
import { openPreview } from "./preview.js";

let log: vscode.OutputChannel;

export function activate(context: vscode.ExtensionContext): void {
  log = vscode.window.createOutputChannel("ConnectView");

  context.subscriptions.push(
    vscode.commands.registerCommand("connectview.openPreview", () => {
      openPreview(context, log);
    })
  );

  log.appendLine("ConnectView extension activated.");
}

export function deactivate(): void {
  log?.dispose();
}
