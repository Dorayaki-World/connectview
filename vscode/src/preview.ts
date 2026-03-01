import * as vscode from "vscode";
import { loadConfig } from "./config.js";
import { compile } from "./protoc.js";

let panel: vscode.WebviewPanel | undefined;
let watcher: vscode.Disposable | undefined;
let refreshTimer: ReturnType<typeof setTimeout> | undefined;

const DEBOUNCE_MS = 500;

export function openPreview(
  context: vscode.ExtensionContext,
  log: vscode.OutputChannel
): void {
  if (panel) {
    panel.reveal(vscode.ViewColumn.Beside);
    return;
  }

  panel = vscode.window.createWebviewPanel(
    "connectview.preview",
    "ConnectView",
    vscode.ViewColumn.Beside,
    { enableScripts: true, retainContextWhenHidden: true }
  );

  panel.onDidDispose(
    () => {
      panel = undefined;
      watcher?.dispose();
      watcher = undefined;
    },
    null,
    context.subscriptions
  );

  // Watch .proto file saves and refresh.
  watcher = vscode.workspace.onDidSaveTextDocument((doc) => {
    if (doc.fileName.endsWith(".proto")) {
      scheduleRefresh(log);
    }
  });

  refresh(log);
}

function scheduleRefresh(log: vscode.OutputChannel): void {
  if (refreshTimer) clearTimeout(refreshTimer);
  refreshTimer = setTimeout(() => refresh(log), DEBOUNCE_MS);
}

async function refresh(log: vscode.OutputChannel): Promise<void> {
  if (!panel) return;

  panel.webview.html = loadingHtml();

  const config = loadConfig();
  log.appendLine(`Compiling protos from ${config.protoRoot} ...`);

  try {
    const result = await compile(config);
    if (!panel) return; // disposed during compile

    if (result.ok) {
      panel.webview.html = result.html;
      log.appendLine("Preview updated.");
    } else {
      panel.webview.html = errorHtml(result.error);
      log.appendLine(`Compile error: ${result.error}`);
    }
  } catch (err: unknown) {
    if (!panel) return;
    const msg = err instanceof Error ? err.message : String(err);
    panel.webview.html = errorHtml(msg);
    log.appendLine(`Error: ${msg}`);
  }
}

function loadingHtml(): string {
  return `<!DOCTYPE html>
<html><body style="display:flex;justify-content:center;align-items:center;height:100vh;
  font-family:system-ui;color:#888">
  <p>Compiling .proto files&#8230;</p>
</body></html>`;
}

function errorHtml(message: string): string {
  const escaped = message
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
  return `<!DOCTYPE html>
<html><body style="padding:2rem;font-family:system-ui">
  <h2 style="color:#e53e3e">ConnectView: Compilation Error</h2>
  <pre style="background:#1a1a2e;color:#e2e8f0;padding:1rem;border-radius:8px;
    overflow-x:auto;white-space:pre-wrap">${escaped}</pre>
</body></html>`;
}
