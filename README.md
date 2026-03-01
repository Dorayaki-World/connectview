# connectview

Interactive API explorer for [ConnectRPC](https://connectrpc.com) services, generated from your proto files.

## Features

- **Self-contained HTML output** — single file, no CDN or external dependencies
- **Live serve mode** — hot reload on proto changes with built-in reverse proxy
- **Try-it panel** — send requests to your services directly from the browser
- **Full proto3 support** — nested messages, enums, oneofs, maps, optional fields, recursive types

## Install

```sh
go install github.com/Dorayaki-World/connectview/cmd/connectview@latest
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

## License

[MIT](LICENSE)
