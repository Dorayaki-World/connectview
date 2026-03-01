# ConnectView for VS Code / Cursor

Preview ConnectRPC service definitions with a Swagger UI-like experience, right inside your editor.

## Prerequisites

- `protoc` installed
- `protoc-gen-connectview` installed

```bash
# protoc (Homebrew)
brew install protobuf

# protoc-gen-connectview
go install github.com/Dorayaki-World/connectview/cmd/connectview@latest
cp "$(go env GOPATH)/bin/connectview" "$(go env GOPATH)/bin/protoc-gen-connectview"
```

## Usage

1. `Cmd+Shift+P` → **ConnectView: Open Preview**
2. Save any `.proto` file — the preview refreshes automatically

You can also click the preview icon in the editor title bar when a `.proto` file is open.

## Settings

| Setting | Default | Description |
|---------|---------|-------------|
| `connectview.protocPath` | `protoc` | Path to the protoc binary |
| `connectview.pluginPath` | `protoc-gen-connectview` | Path to the plugin binary |
| `connectview.protoRoot` | (workspace root) | Root directory containing `.proto` files |
| `connectview.includePaths` | `[]` | Additional `-I` include paths for protoc |

Example (`.vscode/settings.json`):

```json
{
  "connectview.protoRoot": "proto",
  "connectview.includePaths": ["third_party/proto"]
}
```

## Development

```bash
cd vscode
npm install
npm run build          # build once
npm run watch          # rebuild on file changes
```

### Local install via VSIX

```bash
npx @vscode/vsce package
# Cmd+Shift+P → "Extensions: Install from VSIX..." → select connectview-0.1.0.vsix
```

### Extension Development Host (F5 debugging)

1. Open the `vscode/` folder in Cursor or VS Code
2. Press F5 → select "Run Extension"
3. In the new window, open the project root and run the command

## Limitations (v0.1)

- "Send Request" does not work due to webview sandbox restrictions
- Only the first workspace folder is used when multiple folders are open
