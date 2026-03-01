# connectview

Interactive API explorer for [ConnectRPC](https://connectrpc.com) services, generated from your proto files.

## Features

- **Self-contained HTML output** — single file, no CDN or external dependencies
- **Live serve mode** — hot reload on proto changes with built-in reverse proxy
- **VS Code / Cursor extension** — preview proto definitions without leaving the editor
- **Try-it panel** — send requests to your services directly from the browser
- **Full proto3 support** — nested messages, enums, oneofs, maps, optional fields, recursive types

## Install

```sh
go install github.com/Dorayaki-World/connectview/cmd/connectview@latest

# protoc needs the binary named protoc-gen-connectview
cp "$(go env GOPATH)/bin/connectview" "$(go env GOPATH)/bin/protoc-gen-connectview"
```

## Quick Start — Generate Mode

Run as a `protoc` plugin to produce a standalone HTML file:

```sh
protoc \
  --connectview_out=. \
  --proto_path=./proto \
  proto/*.proto
```

Then open `index.html` in your browser.

## Quick Start — Serve Mode

Point connectview at your proto directory and a running ConnectRPC server:

```sh
connectview serve \
  --proto ./proto \
  --target http://localhost:8080
```

Open `http://localhost:9000` — the viewer auto-reloads when proto files change, and the built-in proxy forwards requests to your target server.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--proto` | *(required)* | Proto file root directory |
| `--target` | *(required)* | ConnectRPC target URL |
| `--port` | `9000` | Listen port |
| `-I` | — | Additional import paths (repeatable) |

## VS Code / Cursor Extension

Preview ConnectRPC service definitions inside the editor. The extension runs `protoc` + `protoc-gen-connectview` and displays the output in a webview panel. Saving a `.proto` file automatically refreshes the preview.

### Install

- **VS Code**: [Marketplace](https://marketplace.visualstudio.com/items?itemName=dorayaki-world.connectview)
- **Cursor**: [Open VSX](https://open-vsx.org/extension/dorayaki-world/connectview)

Or search for "ConnectView" in the extensions panel.

### Usage

`Cmd+Shift+P` → **ConnectView: Open Preview**, or click the preview icon in the editor title bar when a `.proto` file is open.

### Settings

| Setting | Default | Description |
|---------|---------|-------------|
| `connectview.protocPath` | `protoc` | Path to the protoc binary |
| `connectview.pluginPath` | `protoc-gen-connectview` | Path to the plugin binary |
| `connectview.protoRoot` | *(workspace root)* | Root directory containing `.proto` files |
| `connectview.includePaths` | `[]` | Additional `-I` include paths for protoc |

The extension auto-detects `buf` module cache (`.buf/`) as an include path. For other setups, use `includePaths`.

## License

[MIT](LICENSE)
