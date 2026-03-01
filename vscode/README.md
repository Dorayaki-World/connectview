# ConnectView for VS Code / Cursor

Preview [ConnectRPC](https://connectrpc.com) service definitions with a Swagger UI-like experience, right inside your editor.

## Prerequisites

- [`protoc`](https://github.com/protocolbuffers/protobuf/releases) installed
- [`protoc-gen-connectview`](https://github.com/Dorayaki-World/connectview) installed

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
| `connectview.protoRoot` | *(workspace root)* | Root directory containing `.proto` files |
| `connectview.includePaths` | `[]` | Additional `-I` include paths for protoc |

Example (`.vscode/settings.json`):

```json
{
  "connectview.protoRoot": "proto",
  "connectview.includePaths": ["third_party/proto"]
}
```

### buf projects

The extension auto-detects the buf module cache (`.buf/`) as an include path. No extra configuration is needed for projects using [buf](https://buf.build).

## Limitations (v0.1)

- "Send Request" does not work due to webview sandbox restrictions
- Only the first workspace folder is used when multiple folders are open
