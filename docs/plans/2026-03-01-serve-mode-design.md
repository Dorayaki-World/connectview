# serveモード設計

## 概要

`connectview serve` コマンドでローカルHTTPサーバーを起動し、
proto ファイルからリアルタイムにドキュメントを生成 + CORS 回避プロキシを提供する。

## CLI

```
connectview serve \
  --proto ./proto \                    # proto ルートディレクトリ（必須）
  --target http://localhost:8080 \     # ConnectRPCサーバーURL（必須）
  --port 9000 \                        # listen ポート（デフォルト: 9000）
  -I ./vendor/proto                    # 追加 import パス（複数指定可）
```

## アーキテクチャ

```
HTTP Server (:9000)
├── GET /           → index.html（スキーマ埋め込み、動的生成）
├── GET /schema.json → スキーマ JSON（hot reload 用）
├── GET /events     → SSE（スキーマ更新通知）
└── POST /proxy/*   → target への reverse proxy

FileWatcher (fsnotify)
  → .proto 変更検知
  → protocompile で再パース → 新 IR → 新スキーマ
  → SSE で全クライアントに通知
```

## コンポーネント

### internal/compiler/compiler.go
- protocompile ライブラリで .proto ファイルを直接パース
- FileDescriptorSet → 既存の parser.Parse 相当の処理 → ir.Root
- import パスの解決（--proto と -I で指定されたディレクトリ）

### internal/server/server.go
- net/http で HTTP サーバー起動
- ルーティング: /, /schema.json, /events, /proxy/*
- スキーマを sync.RWMutex で保護して hot reload 対応

### internal/server/proxy.go
- POST /proxy/{service}/{method} → target へ中継
- リクエストヘッダー（Content-Type, Connect-Protocol-Version, Authorization 等）を中継
- レスポンスをそのまま返却
- CORS ヘッダー付与（Access-Control-Allow-Origin: *）

### internal/server/watcher.go
- fsnotify で --proto ディレクトリを再帰監視
- .proto ファイルの変更/作成/削除を検知
- debounce（100ms）で連続変更をまとめる
- 変更検知 → compiler で再パース → スキーマ更新 → SSE 通知

## Hot Reload フロー

1. fsnotify が .proto 変更を検知
2. debounce 後、protocompile で全 .proto を再パース
3. parser → resolver → スキーマ JSON を生成
4. sync.RWMutex でスキーマを差し替え
5. SSE で `{"type":"schema-updated"}` を送信
6. ブラウザ JS が /schema.json を fetch して DOM 更新

## ブラウザ側の変更（app.js）

- serveモード検知: `window.__CONNECTVIEW_SERVE_MODE__` フラグ
- serveモード時は base URL を自動的に同一オリジン `/proxy/` に設定
- SSE リスナーで `/events` に接続、`schema-updated` イベントで再描画
- スキーマ再取得時は `GET /schema.json` から取得

## 依存追加

- `github.com/bufbuild/protocompile` — proto ファイルの直接パース
- `github.com/fsnotify/fsnotify` — ファイル変更検知
