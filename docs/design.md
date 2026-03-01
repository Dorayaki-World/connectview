# connectview 設計書

## 目次

1. [課題定義](#1-課題定義)
2. [プロダクト概要](#2-プロダクト概要)
3. [用語定義](#3-用語定義)
4. [ユーザーストーリー](#4-ユーザーストーリー)
5. [要件定義](#5-要件定義)
6. [アーキテクチャ設計](#6-アーキテクチャ設計)
7. [中間表現（IR）設計](#7-中間表現ir設計)
8. [UI/UX設計](#8-uiux設計)
9. [ConnectRPC リクエスト仕様](#9-connectrpc-リクエスト仕様)
10. [再帰参照の処理設計](#10-再帰参照の処理設計)
11. [ディレクトリ構成](#11-ディレクトリ構成)
12. [テスト設計](#12-テスト設計)

---

## 1. 課題定義

### 1.1 背景

Protocol Buffers（protobuf）は強力なIDL（Interface Definition Language）だが、その可読性には構造的な制約がある。

```proto
// このRPCの入出力を理解するには、
// 別の場所（別ファイルの場合もある）にコードジャンプする必要がある
service UserService {
  rpc GetUser(GetUserRequest) returns (GetUserResponse) {}
  rpc CreateUser(CreateUserRequest) returns (CreateUserResponse) {}
}
```

RPCの定義とそのリクエスト・レスポンスメッセージは分離して書かれることが多く、
全体像を把握するためにはコードジャンプを繰り返す必要がある。
ネストが深い場合は更に複数回のジャンプが必要になる。

ConnectRPC は HTTP/JSON として使えるという強みを持ちながら、
その強みを活かした「試せるドキュメント」が公式には存在しない。

### 1.2 既存ソリューションの限界

| ツール | 課題 |
|--------|------|
| `protoc-gen-doc` | RPC中心のレイアウトではない。リクエスト送信機能がない |
| Buf Schema Registry (BSR) | クラウド依存。プライベートAPIに使いにくい。リクエスト送信機能が限定的 |
| `grpcui` | gRPC Reflection が必要。ConnectRPCのURLパターンではなくgRPC形式 |
| `protoc-gen-connect-openapi` + Swagger UI | OpenAPIという外来フォーマットを経由。ConnectRPCの思想と乖離 |
| Buf Studio | SaaSのみ。セルフホスト不可 |

### 1.3 解決する課題

```
課題1（可読性）
  protoファイルを読む際に、RPCの入出力メッセージを
  その場でインライン展開して確認できない

課題2（試せない）
  ドキュメントを見ながらその場でAPIを試す手段が
  ConnectRPC向けには存在しない（OpenAPI経由以外）

課題3（共有できない）
  外部パートナーや新規参画者にAPIの全体像を
  シンプルな手順で共有できない
```

### 1.4 対象ユーザー

| ユーザー | 主な用途 |
|----------|----------|
| API実装者 | 実装中の全体構造把握、フィールドの確認 |
| APIレビュアー | PRでのprotoレビュー、設計確認 |
| APIコンシューマ（社内） | 使い方の把握、動作確認 |
| APIコンシューマ（外部） | ドキュメントとして渡される静的HTMLの閲覧 |
| QA・テスター | 手動でのAPIテスト |

---

## 2. プロダクト概要

**connectview** は、protoファイルから「RPCを中心としたインタラクティブなHTMLビューワー」を生成するツールである。

- `protoc` プラグイン（`protoc-gen-connectview`）として動作し、`buf generate` にも対応する
- 生成されたHTMLはサーバー不要で閲覧でき、ブラウザ上からConnectRPCのリクエストを直接送信できる
- `connectview serve` モードでは、ローカルHTTPサーバーとして動作し、CORSを回避したプロキシ機能と hot reload を提供する
- 外部CDN・クラウドサービスへの依存がなく、完全オフラインで動作する

### 2.1 動作モード

```
モード1: generate（静的HTML生成）
  buf generate / protoc plugin として動作
  → index.html を出力（CSS/JS全てinline embed）
  → ドキュメント閲覧は常に可能
  → リクエスト送信はサーバー側のCORS設定に依存

モード2: serve（ローカルサーバー）
  connectview serve --proto ./proto --target http://localhost:8080
  → HTTPサーバーを起動（デフォルト :9000）
  → /proxy/{Service}/{Method} でCORSを回避してリクエストを中継
  → protoファイルの変更を検知してスキーマをライブ更新
```

---

## 3. 用語定義

| 用語 | 定義 |
|------|------|
| IR | Internal Representation。FileDescriptorProtoを表示用に変換した中間表現 |
| FQN | Fully Qualified Name。例: `.connectrpc.greet.v1.GreetRequest` |
| RPC | Remote Procedure Call。serviceブロック内のメソッド定義 |
| 再帰参照 | あるメッセージのフィールドが、そのメッセージ自身または祖先のメッセージを参照すること |
| visitedInPath | 再帰検出のために展開ツリーの現在のパス上に存在するFQNの集合 |
| ConnectRPCパス | `/{package}.{ServiceName}/{MethodName}` 形式のHTTPパス |

---

## 4. ユーザーストーリー

### US-01: ドキュメント閲覧（基本）

```
As a APIコンシューマ
I want to protoファイルから生成されたHTMLをブラウザで開いて、
  各RPCのリクエスト・レスポンス構造をその場で確認したい
So that コードジャンプなしにAPIの全体像を把握できる

Acceptance Criteria:
  - RPCの一覧が左サイドバーに表示される
  - 各RPCをクリックすると、Request/Responseのフィールドがインライン展開される
  - ネストされたメッセージも再帰的に展開される
  - protoコメントがドキュメントとして表示される
  - ConnectRPCのHTTPパスが表示される
```

### US-02: リクエスト送信

```
As a 開発者/テスター
I want to ドキュメントを見ながらそのままフォームに値を入力してAPIを試したい
So that 別のツール（Postman等）を開かずにAPIの動作確認ができる

Acceptance Criteria:
  - 各RPCにフォームが自動生成される
  - フォームはprotoの型に対応したUIを持つ（テキスト/数値/チェックボックス/セレクト等）
  - Sendボタンでリクエストを送信できる
  - レスポンスがJSON整形されて表示される
  - ConnectRPCエラーレスポンスが識別・整形されて表示される
  - リクエストのcurlコマンドをコピーできる
```

### US-03: 認証ヘッダー設定

```
As a 開発者
I want to Bearer トークン等のHTTPヘッダーを設定してリクエストを送りたい
So that 認証が必要なAPIもドキュメント上でテストできる

Acceptance Criteria:
  - ヘッダーをkey/value形式で追加できる
  - 設定したヘッダーは全RPCのリクエストに適用される
  - セッション中はヘッダーが保持される
```

### US-04: buf generateとの統合

```
As a bufユーザー
I want to buf.gen.yaml に数行追加するだけでHTMLを生成したい
So that 既存のproto管理ワークフローに最小コストで組み込める

Acceptance Criteria:
  - buf.gen.yaml のpluginとして設定できる
  - buf generate 一コマンドでindex.htmlが生成される
  - BSRのリモートプラグインとしても利用できる
```

### US-05: serveモードでの開発

```
As a API実装者
I want to protoファイルを編集したらドキュメントが自動更新される状態で開発したい
So that 常に最新のスキーマを確認しながら実装を進められる

Acceptance Criteria:
  - connectview serve でローカルサーバーが起動する
  - protoファイルの変更を検知してスキーマが自動更新される
  - CORSの問題なくローカルのConnectRPCサーバーにリクエストを送信できる
```

### US-06: 静的HTMLの共有

```
As a APIプロバイダー
I want to 生成したHTMLファイルを外部パートナーに渡したい
So that パートナーがサーバー等の準備なしにAPIを理解できる

Acceptance Criteria:
  - 生成されたHTMLはシングルファイルで、外部依存がない
  - ファイルをダブルクリックするだけでブラウザで閲覧できる
  - このモードではリクエスト送信のBase URLを自由に変更できる
```

---

## 5. 要件定義

### 5.1 機能要件 MVP（v0.1）

```
FR-01  protoファイルを入力として受け取る（protoc/buf のCodeGeneratorRequest）
FR-02  Serviceを起点にRPCを一覧表示する
FR-03  RPCのRequest/Responseメッセージをインライン展開して表示する
FR-04  ネストされたメッセージを再帰的に展開する（再帰参照は制御する）
FR-05  protoコメントをドキュメントとして表示する
FR-06  ConnectRPCのHTTPパスを表示する
FR-07  staticなシングルHTMLとして出力する（外部依存なし）
FR-08  複数protoファイルにまたがるimportを解決する
FR-09  フィールド型に応じたフォームを自動生成する
FR-10  フォームからConnectRPC形式でPOSTリクエストを送信する
FR-11  レスポンス（正常/エラー）をJSON整形して表示する
FR-12  HTTPヘッダーを追加してリクエストに付与する
FR-13  Base URLを設定できる（デフォルト: http://localhost:8080）
FR-14  curlコマンドスニペットを生成してコピーできる
FR-15  serveモード: ローカルHTTPサーバーとして動作する
FR-16  serveモード: /proxy エンドポイントでConnectRPCサーバーへ中継する
FR-17  serveモード: protoファイルの変更を検知してスキーマを更新する
```

### 5.2 機能要件 v0.2以降

```
FR-18  フィールド・RPC名での検索・フィルタリング
FR-19  リクエスト履歴の表示（セッション内）
FR-20  protovalidate制約（buf.validate.field）の表示
FR-21  Enum値の展開表示
FR-22  google.api.http アノテーションのREST URLパス表示
FR-23  Server Streaming RPCの逐次レスポンス表示
FR-24  Mermaid形式でのメッセージ依存グラフ出力
FR-25  ダークモード対応
```

### 5.3 非機能要件

```
NFR-01  クラウド依存なし（完全オフライン動作）
NFR-02  生成HTMLは外部JS/CSS/フォント依存なし（完全自己完結）
NFR-03  Goのシングルバイナリで配布できる
NFR-04  protoc plugin と buf generate plugin の両方として動作する
NFR-05  生成速度: 100 protoファイルで < 1秒
NFR-06  生成HTMLサイズ: 基本的なAPIで < 500KB
NFR-07  ブラウザ対応: モダンブラウザ（Chrome/Firefox/Safari/Edge 最新2バージョン）
NFR-08  Go 1.22以上で動作する
```

### 5.4 制約・スコープ外

```
OUT-OF-SCOPE:
  - proto2 構文の完全サポート（proto3を主対象とする）
  - gRPCバイナリプロトコルでの送信（ConnectRPC JSON のみ）
  - Client Streaming RPC のフォーム入力（Server Streaming はv0.2で対応）
  - 認証情報のlocalStorage永続化（セキュリティリスク回避）
  - BSRとの直接連携
```

---

## 6. アーキテクチャ設計

### 6.1 全体フロー

```
┌─────────────────────────────────────────────────────────────┐
│  generateモード                                              │
│                                                             │
│  .proto files                                               │
│       ↓  buf generate / protoc                              │
│  CodeGeneratorRequest (stdin)                               │
│       ↓  internal/parser                                    │
│  IR (Services / Messages / Enums)                           │
│       ↓  internal/resolver                                  │
│  ResolvedIR (cross-file参照解決済み)                         │
│       ↓  internal/renderer                                  │
│  index.html (CSS/JS全てinline)                               │
│       ↓                                                     │
│  ブラウザで直接開く                                           │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│  serveモード                                                 │
│                                                             │
│  connectview serve --proto ./proto --target http://localhost:8080
│       ↓                                                     │
│  HTTP Server (default :9000)                                │
│  ├── GET  /              → index.html (動的生成)             │
│  ├── GET  /schema.json   → IR JSON                          │
│  ├── GET  /events        → SSE (スキーマ更新通知)            │
│  └── POST /proxy/{Svc}/{Method}                             │
│              ↓ リクエスト中継                                │
│         ConnectRPC Server                                   │
│                                                             │
│  FileWatcher → protoファイル変更検知 → スキーマ再生成         │
│             → SSE でブラウザに通知                           │
└─────────────────────────────────────────────────────────────┘
```

### 6.2 パッケージ構成と責務

```
cmd/connectview/
  main.go             エントリポイント。generate / serve の2モード切替

internal/parser/
  parser.go           protogen.Plugin → IR変換（コメント抽出・map検出・optional検出含む）

internal/resolver/
  resolver.go         cross-fileメッセージ・enum参照の解決、再帰参照の検出

internal/renderer/
  renderer.go         ResolvedIR → HTMLテンプレート適用
  embed.go            //go:embed でassets をバイナリに埋め込む

internal/renderer/assets/
  index.html.tmpl     メインHTMLテンプレート
  style.css           スタイルシート
  app.js              フォーム生成・リクエスト送信ロジック

internal/compiler/
  compiler.go         protocompile で .proto ファイルを直接パース → protogen.Plugin → parser.Parse() で IR 生成

internal/server/         （serveモード）
  server.go           serveモードのHTTPサーバー（SSE 対応）
  proxy.go            /proxy エンドポイントの実装
  watcher.go          protoファイルのFileWatcher（fsnotify, 100ms debounce）
```

※ `internal/plugin/` と `internal/parser/comment.go` は不要。
  `google.golang.org/protobuf/compiler/protogen` が CodeGeneratorRequest/Response の
  I/O処理、SourceCodeInfoからのコメント抽出、map検出、proto3 optional検出を全て担う。

### 6.3 protoc plugin インターフェース

```go
// cmd/connectview/main.go
// protogen を使用。stdin/stdout の処理は protogen が自動で行う。

func main() {
    var flags flag.FlagSet
    protogen.Options{
        ParamFunc: flags.Set,
    }.Run(func(plugin *protogen.Plugin) error {
        plugin.SupportedFeatures = uint64(
            pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL,
        )

        root := parser.Parse(plugin)

        r := resolver.New(root)
        if err := r.Resolve(); err != nil {
            return err
        }

        html, err := renderer.New().Render(root)
        if err != nil {
            return err
        }

        outFile := plugin.NewGeneratedFile("index.html", "")
        _, err = outFile.Write([]byte(html))
        return err
    })
}
```

---

## 7. 中間表現（IR）設計

### 7.1 IR型定義

```go
// internal/ir/ir.go

package ir

// Root は全体のIR。Rendererに渡す最上位の構造体
type Root struct {
    Files    []*File
    Services []*Service
    // FQN（"."始まり）→ Message のルックアップテーブル
    // Resolverがcross-file参照の解決に使用する
    Messages map[string]*Message
    Enums    map[string]*Enum
}

type File struct {
    Name    string // "greet/v1/greet.proto"
    Package string // "connectrpc.greet.v1"
}

type Service struct {
    Name        string   // "GreetService"
    FullName     string   // "connectrpc.greet.v1.GreetService"
    File         string   // "greet/v1/greet.proto"
    Comment      string
    RPCs         []*RPC
    // ConnectRPCのベースパス
    // "/{FullName}/"
    ConnectBasePath string
}

type RPC struct {
    Name             string // "Greet"
    Comment          string
    ConnectPath      string // "/connectrpc.greet.v1.GreetService/Greet"
    HTTPMethod       string // "POST" または "GET"（idempotency_level=NO_SIDE_EFFECTSの場合）
    Request          *MessageRef
    Response         *MessageRef
    ClientStreaming  bool
    ServerStreaming  bool
}

type MessageRef struct {
    // protoファイル上の型名（"."始まりのFQN）
    // 例: ".connectrpc.greet.v1.GreetRequest"
    TypeName string
    // Resolverが解決した実体（nilの場合は未解決）
    Resolved *Message
}

type Message struct {
    Name      string // "GreetRequest"
    FullName  string // ".connectrpc.greet.v1.GreetRequest"
    Comment   string
    Fields    []*Field
    // ネストされたメッセージ定義（Message内Messageの場合）
    NestedMessages []*Message
    NestedEnums    []*Enum
    // synthetic map entry message
    IsMapEntry bool
}

type Field struct {
    Name    string
    Number  int32
    Type    FieldType
    // MESSAGEまたはENUMの場合のFQN（"."始まり）
    TypeName string
    Label    FieldLabel
    Comment  string
    // proto3 optional キーワード
    IsOptional bool
    // oneofグループ名（real oneofに属する場合。synthetic oneofは含まない）
    OneofName string
    // map<K,V> フィールドのサポート
    IsMap            bool
    MapKeyType       FieldType
    MapValueType     FieldType
    MapValueTypeName string // map value が MESSAGE/ENUM の場合の FQN
    // Resolverが解決した実体
    ResolvedMessage *Message
    ResolvedEnum    *Enum
    // 再帰参照フラグ（Resolverが設定）
    IsRecursive bool
}

type FieldType int32

const (
    FieldTypeDouble   FieldType = 1
    FieldTypeFloat    FieldType = 2
    FieldTypeInt64    FieldType = 3
    FieldTypeUint64   FieldType = 4
    FieldTypeInt32    FieldType = 5
    FieldTypeFixed64  FieldType = 6
    FieldTypeFixed32  FieldType = 7
    FieldTypeBool     FieldType = 8
    FieldTypeString   FieldType = 9
    FieldTypeBytes    FieldType = 12
    FieldTypeUint32   FieldType = 13
    FieldTypeEnum     FieldType = 14
    FieldTypeSfixed32 FieldType = 15
    FieldTypeSfixed64 FieldType = 16
    FieldTypeSint32   FieldType = 17
    FieldTypeSint64   FieldType = 18
    FieldTypeMessage  FieldType = 11
)

type FieldLabel int32

const (
    FieldLabelOptional FieldLabel = 1
    FieldLabelRequired FieldLabel = 2
    FieldLabelRepeated FieldLabel = 3
)

type Enum struct {
    Name     string // "UserStatus"
    FullName string // ".connectrpc.user.v1.UserStatus"
    Comment  string
    Values   []*EnumValue
}

type EnumValue struct {
    Name    string // "USER_STATUS_ACTIVE"
    Number  int32  // 1
    Comment string
}
```

### 7.2 Parser の変換ルール

```go
// internal/parser/parser.go
// protogen を使用。コメント抽出・map検出・optional検出は protogen が自動で行う。

// Parse は protogen.Plugin のファイルを IR に変換する
func Parse(plugin *protogen.Plugin) *ir.Root {
    root := &ir.Root{
        Messages: make(map[string]*ir.Message),
        Enums:    make(map[string]*ir.Enum),
    }
    for _, f := range plugin.Files {
        if !f.Generate {
            continue
        }
        // messages, enums, services を変換
    }
    return root
}

// protogen.Service → ir.Service
func parseService(svc *protogen.Service) *ir.Service {
    fullName := string(svc.Desc.FullName())
    return &ir.Service{
        Name:            string(svc.Desc.Name()),
        FullName:        fullName,
        File:            svc.Desc.ParentFile().Path(),
        Comment:         cleanComment(string(svc.Comments.Leading)),
        ConnectBasePath: "/" + fullName + "/",
    }
}

// protogen.Method → ir.RPC
// idempotency_level は method.Desc.Options() から取得
// コメントは method.Comments.Leading から取得（SourceCodeInfo パース不要）
// map フィールドは field.Desc.IsMap() で検出
// proto3 optional は field.Desc.HasOptionalKeyword() で検出
// synthetic oneof は field.Oneof.Desc.IsSynthetic() で判別
```

### 7.3 Resolver の動作

```go
// internal/resolver/resolver.go

type Resolver struct {
    root *ir.Root
}

// Resolve は Root.Messages / Root.Enums を参照して
// 全 MessageRef.Resolved を埋め、再帰参照を検出してマークする
func (r *Resolver) Resolve() error {
    for _, svc := range r.root.Services {
        for _, rpc := range svc.RPCs {
            if err := r.resolveMessageRef(rpc.Request, nil); err != nil {
                return err
            }
            if err := r.resolveMessageRef(rpc.Response, nil); err != nil {
                return err
            }
        }
    }
    return nil
}

// resolveMessageRef は visitedInPath でパス上の祖先FQNを管理する。
// 同じFQNがパス上に2回現れた時点で IsRecursive = true を設定し、展開を停止する。
// 兄弟フィールドで同じ型が複数回現れる場合は再帰ではないので正常展開する。
func (r *Resolver) resolveMessageRef(ref *ir.MessageRef, visitedInPath map[string]bool) error {
    msg, ok := r.root.Messages[ref.TypeName]
    if !ok {
        return fmt.Errorf("message not found: %s", ref.TypeName)
    }
    ref.Resolved = msg

    if visitedInPath == nil {
        visitedInPath = make(map[string]bool)
    }

    for _, field := range msg.Fields {
        if field.Type == ir.FieldTypeMessage {
            if visitedInPath[field.TypeName] {
                // パス上に既に存在 → 再帰参照
                field.IsRecursive = true
                // ResolvedMessage は型情報として設定する（展開はしない）
                field.ResolvedMessage = r.root.Messages[field.TypeName]
                continue
            }
            // 現在のパスにFQNを追加してから再帰
            childPath := copyMap(visitedInPath)
            childPath[ref.TypeName] = true
            childRef := &ir.MessageRef{TypeName: field.TypeName}
            if err := r.resolveMessageRef(childRef, childPath); err != nil {
                return err
            }
            field.ResolvedMessage = childRef.Resolved
        } else if field.Type == ir.FieldTypeEnum {
            field.ResolvedEnum = r.root.Enums[field.TypeName]
        }
    }
    return nil
}
```

---

## 8. UI/UX設計

### 8.1 レイアウト構造

```
┌─────────────────────────────────────────────────────────────────────┐
│  HEADER                                                             │
│  connectview  [🔍 filter...]          [Base URL: localhost:8080] [⚙]  │
├─────────────────┬───────────────────────────────────────────────────┤
│  SIDEBAR        │  MAIN CONTENT                                     │
│                 │                                                   │
│  package:       │  ── Service Name ──────────────────────────────   │
│  greet.v1       │  package: connectrpc.greet.v1                     │
│                 │  comment (if any)                                 │
│  ▼ GreetService │                                                   │
│    ├─ Greet     │  ┌── RPC card ───────────────────────────────┐    │
│    └─ GreetAll  │  │  [POST] /connectrpc.greet.v1.../Greet      │    │
│                 │  │  comment                                   │    │
│  package:       │  │                                            │    │
│  user.v1        │  │  REQUEST  GreetRequest                     │    │
│                 │  │  ├─ name: string         // comment        │    │
│  ▼ UserService  │  │  └─ locale?: string                        │    │
│    └─ GetUser   │  │                                            │    │
│                 │  │  RESPONSE  GreetResponse                   │    │
│                 │  │  └─ greeting: string                       │    │
│                 │  │                                            │    │
│                 │  │  [▶ Try it]                                │    │
│                 │  └────────────────────────────────────────────┘    │
│                 │                                                   │
│                 │  ┌── TRY IT PANEL (展開時) ───────────────────┐    │
│                 │  │  Headers  [+ Add Header]                   │    │
│                 │  │  ┌────────────────────────────────────┐    │    │
│                 │  │  │ Authorization  Bearer ___________  │    │    │
│                 │  │  └────────────────────────────────────┘    │    │
│                 │  │                                            │    │
│                 │  │  Request Body                              │    │
│                 │  │  ┌── GreetRequest ─────────────────────┐  │    │
│                 │  │  │  name    [________________]          │  │    │
│                 │  │  │  locale  [________________]          │  │    │
│                 │  │  └─────────────────────────────────────┘  │    │
│                 │  │                                            │    │
│                 │  │          [Send ▶]  [Copy as curl 📋]      │    │
│                 │  │                                            │    │
│                 │  │  Response  ── 200 OK (42ms) ──────────    │    │
│                 │  │  {                                         │    │
│                 │  │    "greeting": "Hello, World!"             │    │
│                 │  │  }                                         │    │
│                 │  └────────────────────────────────────────────┘    │
└─────────────────┴───────────────────────────────────────────────────┘
```

### 8.2 フィールド表示仕様

#### ドキュメントビュー（RPC Card内）

```
// スカラー型
name: string                    // コメント
age: int32
score: float

// optional（proto3 optional）
email?: string

// repeated
tags: string[]
items: OrderItem[]

// map
metadata: map<string, string>

// ネストされたメッセージ（インライン展開）
address: Address
  ├─ street: string
  ├─ city: string
  └─ country: string

// enum（値一覧表示）
status: UserStatus
  ├─ USER_STATUS_UNSPECIFIED = 0
  ├─ USER_STATUS_ACTIVE = 1
  └─ USER_STATUS_INACTIVE = 2

// oneof
[oneof: contact]
  ├─ phone: string
  └─ email: string

// 再帰参照（展開停止）
parent?: Node    ↻  // [recursive]
children: Node[] ↻

// Well-Known Types
created_at: google.protobuf.Timestamp
updated_at: google.protobuf.Timestamp
```

#### フィールドのビジュアルスタイル

```css
/* 型名: 緑 */
.field-type { color: #2e7d32; }

/* コメント: グレー（GitHub風） */
.field-comment { color: #6a737d; }

/* optional の ? マーク: グレー */
.field-optional { color: #9e9e9e; }

/* repeated の [] : 青 */
.field-repeated { color: #1565c0; }

/* 再帰参照の ↻ : オレンジ */
.field-recursive { color: #e65100; }

/* oneof グループ: 紫のボーダー */
.field-oneof { border-left: 3px solid #7b1fa2; }
```

### 8.3 フォームUI仕様（Try It Panel）

#### 型ごとのフォームコンポーネント

```
proto型                 → フォームUI
─────────────────────────────────────────────────────
string                 → <input type="text">
int32 / sint32         → <input type="number" step="1">
int64 / sint64         → <input type="text"> ※proto JSONではstring
uint32                 → <input type="number" step="1" min="0">
uint64                 → <input type="text"> ※proto JSONではstring
float                  → <input type="number" step="any">
double                 → <input type="number" step="any">
bool                   → <input type="checkbox">
bytes                  → <input type="text"> + base64ヒント表示
enum                   → <select> (値名と番号を表示)
message（ネスト）       → フィールドグループ（折りたたみ可能）
repeated scalar        → タグ入力UI（Enterで追加、×で削除）
repeated message       → [+ Add Item] ボタンで動的にフォーム追加
map<string, V>         → key/valueペア入力（動的追加）
oneof                  → ラジオボタンで選択 → 選択フィールドのみ表示
google.protobuf.Timestamp → <input type="datetime-local">
google.protobuf.Duration  → seconds/nanosのペア入力
google.protobuf.StringValue（Wrapper型） → <input type="text"> + null toggle
```

#### フォームのJSON変換ルール

```javascript
// RequestBuilder: フォームDOM状態 → ConnectRPC JSON body

// 空文字列フィールドはデフォルト値として送信しない
// （proto3のデフォルト値はフィールドなしと同等）
// ただし、ユーザーが明示的に値を入力した場合は送信する

// int64/uint64 はstringとして送信（proto JSON mapping）
// 例: { "id": "9007199254740993" }

// bytes はbase64エンコードして送信
// 例: { "data": "aGVsbG8=" }

// Timestamp はRFC3339形式のstringとして送信
// 例: { "created_at": "2024-01-01T00:00:00Z" }

// repeated が空の場合は送信しない（空配列でも可だが省略を優先）

// oneofは選択されているフィールドのみ送信
```

### 8.4 レスポンス表示仕様

```javascript
// 正常時 (2xx)
// ステータス: 緑 "200 OK (42ms)"
// ボディ: JSON prettify + シンタックスハイライト

// ConnectRPCエラー時 (非2xx)
// ステータス: 赤 "400 Bad Request"
// エラー表示:
{
  "code": "invalid_argument",
  "message": "name is required",
  "details": [...]
}
// code フィールドは ConnectRPCエラーコード対応表でhuman-readableに変換

// ネットワークエラー時
// "Network Error: Failed to fetch （CORSの可能性があります）"
// → serveモードの使用を促すメッセージ

// タイムアウト（10秒）
// "Request timed out after 10s"
```

### 8.5 curl スニペット生成仕様

```javascript
// curlスニペットの生成例

// 通常のPOST
curl -X POST \
  -H "Content-Type: application/json" \
  -H "Connect-Protocol-Version: 1" \
  -H "Authorization: Bearer ..." \
  -d '{"name":"World"}' \
  http://localhost:8080/connectrpc.greet.v1.GreetService/Greet

// idempotency_level=NO_SIDE_EFFECTS のGET
curl "http://localhost:8080/connectrpc.greet.v1.GreetService/Greet?encoding=json&message=%7B%22name%22%3A%22World%22%7D"
```

---

## 9. ConnectRPC リクエスト仕様

### 9.1 Unary RPC（POST）

```javascript
// app.js 内の RPCSender

async function sendUnary(rpc, requestBody, headers, baseURL) {
    const url = `${baseURL}${rpc.connectPath}`;

    const response = await fetch(url, {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
            "Connect-Protocol-Version": "1",
            ...headers  // ユーザー設定ヘッダー
        },
        body: JSON.stringify(requestBody),
        signal: AbortSignal.timeout(10000)  // 10秒タイムアウト
    });

    const data = await response.json();

    if (!response.ok) {
        // ConnectRPCエラーレスポンス
        throw new ConnectError(response.status, data);
    }
    return data;
}
```

### 9.2 Unary RPC（GET）

```javascript
// idempotency_level = NO_SIDE_EFFECTS の場合
async function sendUnaryGet(rpc, requestBody, headers, baseURL) {
    const encoded = encodeURIComponent(JSON.stringify(requestBody));
    const url = `${baseURL}${rpc.connectPath}?encoding=json&message=${encoded}`;

    const response = await fetch(url, {
        method: "GET",
        headers: {
            "Connect-Protocol-Version": "1",
            ...headers
        },
        signal: AbortSignal.timeout(10000)
    });

    return await response.json();
}
```

### 9.3 Server Streaming（v0.2）

```javascript
// Server Streaming は fetch + ReadableStream で実装予定
// ConnectRPC streaming protocol:
// Content-Type: application/connect+json
// レスポンスは enveloped message 形式

async function sendServerStream(rpc, requestBody, headers, baseURL, onMessage) {
    // v0.2 で実装
}
```

### 9.4 serveモードのプロキシエンドポイント

```go
// internal/server/proxy.go

// Proxy は --target で指定されたConnectRPCサーバーへリクエストを中継する。
// /proxy/ プレフィックスを除去し、そのままtargetへ転送する。
// 全レスポンスに CORS ヘッダーを付与。

type Proxy struct {
    targetURL string
    client    *http.Client
}

func NewProxy(targetURL string) *Proxy
func (p *Proxy) Handler() http.Handler

// POST /proxy/connectrpc.greet.v1.GreetService/Greet
//   → http://target:port/connectrpc.greet.v1.GreetService/Greet
// Content-Type, Authorization, Connect-Protocol-Version 等のヘッダーを中継
// OPTIONS → 204 (CORS preflight)
```

---

## 10. 再帰参照の処理設計

### 10.1 パターン分類

```proto
// パターン1: 直接自己参照
message TreeNode {
    string value = 1;
    repeated TreeNode children = 2;  // 自分自身を参照
}

// パターン2: 相互再帰
message A {
    B b = 1;
}
message B {
    A a = 1;  // AがBを参照し、BがAを参照
}

// パターン3: 間接再帰
message X { Y y = 1; }
message Y { Z z = 1; }
message Z { X x = 1; }  // X→Y→Z→X

// パターン4: 非再帰（同じ型が兄弟に複数）
message Response {
    User created_by = 1;  // ← 展開する
    User updated_by = 2;  // ← 展開する（再帰ではない）
}
```

### 10.2 アルゴリズム詳細

```
expandMessage(message M, visitedInPath P):

  入力: M = 展開対象のメッセージ
       P = 現在の展開パス上にある祖先メッセージのFQN集合

  for each field F in M.fields:
    if F.type == MESSAGE:
      if F.typeName in P:
        // パス上に既に存在 → 再帰
        F.isRecursive = true
        F.resolvedMessage = lookup(F.typeName)  // 型情報は設定（展開はしない）
      else:
        // パスに現在のメッセージMのFQNを加えて再帰
        newPath = P ∪ {M.fullName}
        expandMessage(lookup(F.typeName), newPath)
        F.resolvedMessage = lookup(F.typeName)

注意:
  - P はコピーして渡す（兄弟フィールドのパスが干渉しないよう）
  - M.fullName を追加するのは「Mの子を展開する前」
  - これにより、Mのフィールドがまた M を参照した場合に再帰として検出される
```

### 10.3 UI上の再帰参照表示

```
// 通常の展開（非再帰）
Response
├─ user: User
│   ├─ id: string
│   └─ name: string
└─ owner: User         ← 兄弟で同じ型 → 正常展開
    ├─ id: string
    └─ name: string

// 再帰参照（展開停止）
TreeNode
├─ value: string
└─ children: TreeNode[]  ↻  [展開する ▶]
                             ↑ クリックで1段階だけ強制展開

// 強制展開時（1段階のみ）
TreeNode
├─ value: string
└─ children: TreeNode[]
    ├─ value: string
    └─ children: TreeNode[]  ↻  [展開する ▶]  ← 再び停止
```

---

## 11. ディレクトリ構成

```
connectview/
├── cmd/
│   └── connectview/
│       ├── main.go
│       ├── cmd_generate.go
│       └── cmd_serve.go
│
├── internal/
│   ├── ir/
│   │   └── ir.go                    # IR型定義
│   ├── parser/
│   │   ├── parser.go                # FileDescriptorProto → IR
│   │   ├── parser_test.go
│   │   └── comment.go               # SourceCodeInfoからコメント抽出
│   ├── resolver/
│   │   ├── resolver.go              # cross-file参照解決
│   │   ├── resolver_test.go
│   │   └── recursive.go             # 再帰参照検出
│   ├── renderer/
│   │   ├── renderer.go              # IR → HTML
│   │   ├── renderer_test.go
│   │   ├── embed.go                 # go:embed
│   │   └── assets/
│   │       ├── index.html.tmpl
│   │       ├── style.css
│   │       └── app.js
│   └── server/
│       ├── server.go
│       ├── proxy.go
│       └── watcher.go
│
├── testdata/
│   ├── proto/
│   │   ├── greet/v1/greet.proto     # 基本ケース
│   │   ├── user/v1/user.proto       # enum / optional / nested
│   │   ├── tree/v1/tree.proto       # 再帰参照
│   │   ├── mutual/v1/mutual.proto   # 相互再帰
│   │   └── wkt/v1/wkt.proto         # Well-Known Types
│   └── expected/
│       ├── greet.ir.json            # パーサー期待出力（IR JSON）
│       ├── user.ir.json
│       └── tree.ir.json
│
├── go.mod
├── go.sum
├── buf.gen.yaml                     # 自身のtestdata生成用
└── README.md
```

---

## 12. テスト設計

### 12.1 テスト対象とテスト種別

```
Layer               テスト種別        カバー対象
─────────────────────────────────────────────────────────
parser              Unit              FileDescriptorProto → IR変換
resolver            Unit              参照解決・再帰検出アルゴリズム
renderer            Golden File       IR → HTML出力の回帰テスト
server/proxy        Integration       HTTPプロキシの動作
e2e                 End-to-End        protocコマンドによる全体フロー
```

### 12.2 テスト用 proto ファイル

#### testdata/proto/greet/v1/greet.proto（基本ケース）

```proto
syntax = "proto3";

package connectrpc.greet.v1;

option go_package = "connectrpc/greet/v1;greetv1";

// GreetService provides greeting functionality.
service GreetService {
  // Greet sends a greeting to the named subject.
  rpc Greet(GreetRequest) returns (GreetResponse) {}

  // GreetNoResponse is a one-way RPC with no side effects.
  rpc GreetNoResponse(GreetRequest) returns (GreetResponse) {
    option idempotency_level = NO_SIDE_EFFECTS;
  }
}

// GreetRequest contains the subject to greet.
message GreetRequest {
  // The subject to greet.
  string name = 1;
  // The locale for the greeting. Optional.
  optional string locale = 2;
}

// GreetResponse contains the greeting message.
message GreetResponse {
  // The greeting.
  string greeting = 1;
}
```

#### testdata/proto/user/v1/user.proto（enum / nested / repeated）

```proto
syntax = "proto3";

package user.v1;

option go_package = "user/v1;userv1";

// UserService manages users.
service UserService {
  // GetUser retrieves a user by ID.
  rpc GetUser(GetUserRequest) returns (GetUserResponse) {}
  // ListUsers retrieves a list of users.
  rpc ListUsers(ListUsersRequest) returns (ListUsersResponse) {}
}

message GetUserRequest {
  string user_id = 1;
}

message GetUserResponse {
  User user = 1;
}

message ListUsersRequest {
  int32 page_size = 1;
  string page_token = 2;
  // Filter by status.
  UserStatus status_filter = 3;
}

message ListUsersResponse {
  repeated User users = 1;
  string next_page_token = 2;
}

// User represents a user account.
message User {
  string id = 1;
  string name = 2;
  optional string email = 3;
  // Current account status.
  UserStatus status = 4;
  // User's address.
  Address address = 5;
  // Tags for categorization.
  repeated string tags = 6;
  // Metadata key-value pairs.
  map<string, string> metadata = 7;

  oneof contact {
    string phone = 8;
    string slack_id = 9;
  }
}

// Address represents a mailing address.
message Address {
  string street = 1;
  string city = 2;
  string country = 3;
  optional string zip = 4;
}

// UserStatus represents account status.
enum UserStatus {
  USER_STATUS_UNSPECIFIED = 0;
  USER_STATUS_ACTIVE = 1;
  USER_STATUS_INACTIVE = 2;
  USER_STATUS_SUSPENDED = 3;
}
```

#### testdata/proto/tree/v1/tree.proto（再帰参照）

```proto
syntax = "proto3";

package tree.v1;

option go_package = "tree/v1;treev1";

service TreeService {
  rpc GetTree(GetTreeRequest) returns (GetTreeResponse) {}
}

message GetTreeRequest {
  string root_id = 1;
}

message GetTreeResponse {
  TreeNode root = 1;
}

// TreeNode is a node in a tree structure.
// children is a recursive reference to TreeNode.
message TreeNode {
  string id = 1;
  string label = 2;
  // Direct children of this node.
  repeated TreeNode children = 3;
  // Parent node. Optional.
  optional TreeNode parent = 4;
}
```

#### testdata/proto/mutual/v1/mutual.proto（相互再帰）

```proto
syntax = "proto3";

package mutual.v1;

option go_package = "mutual/v1;mutualv1";

service MutualService {
  rpc Get(GetRequest) returns (GetResponse) {}
}

message GetRequest {
  NodeA a = 1;
}

message GetResponse {
  NodeA result = 1;
}

// NodeA and NodeB are mutually recursive.
message NodeA {
  string value = 1;
  NodeB b = 2;
}

message NodeB {
  string value = 1;
  NodeA a = 2;  // mutually recursive with NodeA
}
```

### 12.3 Parser ユニットテスト

```go
// internal/parser/parser_test.go

package parser_test

import (
    "testing"

    "github.com/yourorg/connectview/internal/ir"
    "github.com/yourorg/connectview/internal/parser"
    "google.golang.org/protobuf/proto"
    descriptorpb "google.golang.org/protobuf/types/descriptorpb"
)

func TestParseService_Basic(t *testing.T) {
    // greet.proto の FileDescriptorProto を手動構築してテスト
    fd := &descriptorpb.FileDescriptorProto{
        Name:    proto.String("greet/v1/greet.proto"),
        Package: proto.String("connectrpc.greet.v1"),
        Service: []*descriptorpb.ServiceDescriptorProto{
            {
                Name: proto.String("GreetService"),
                Method: []*descriptorpb.MethodDescriptorProto{
                    {
                        Name:       proto.String("Greet"),
                        InputType:  proto.String(".connectrpc.greet.v1.GreetRequest"),
                        OutputType: proto.String(".connectrpc.greet.v1.GreetResponse"),
                    },
                },
            },
        },
    }

    root, err := parser.Parse([]*descriptorpb.FileDescriptorProto{fd})
    if err != nil {
        t.Fatalf("Parse failed: %v", err)
    }

    if len(root.Services) != 1 {
        t.Errorf("expected 1 service, got %d", len(root.Services))
    }

    svc := root.Services[0]
    if svc.Name != "GreetService" {
        t.Errorf("expected service name GreetService, got %s", svc.Name)
    }
    if svc.FullName != "connectrpc.greet.v1.GreetService" {
        t.Errorf("unexpected fullName: %s", svc.FullName)
    }
    if svc.ConnectBasePath != "/connectrpc.greet.v1.GreetService/" {
        t.Errorf("unexpected ConnectBasePath: %s", svc.ConnectBasePath)
    }

    if len(svc.RPCs) != 1 {
        t.Fatalf("expected 1 RPC, got %d", len(svc.RPCs))
    }

    rpc := svc.RPCs[0]
    if rpc.Name != "Greet" {
        t.Errorf("expected rpc name Greet, got %s", rpc.Name)
    }
    if rpc.ConnectPath != "/connectrpc.greet.v1.GreetService/Greet" {
        t.Errorf("unexpected ConnectPath: %s", rpc.ConnectPath)
    }
    if rpc.HTTPMethod != "POST" {
        t.Errorf("expected HTTPMethod POST, got %s", rpc.HTTPMethod)
    }
    if rpc.Request.TypeName != ".connectrpc.greet.v1.GreetRequest" {
        t.Errorf("unexpected request typeName: %s", rpc.Request.TypeName)
    }
}

func TestParseRPC_IdempotencyLevel_GET(t *testing.T) {
    fd := &descriptorpb.FileDescriptorProto{
        Name:    proto.String("greet/v1/greet.proto"),
        Package: proto.String("connectrpc.greet.v1"),
        Service: []*descriptorpb.ServiceDescriptorProto{
            {
                Name: proto.String("GreetService"),
                Method: []*descriptorpb.MethodDescriptorProto{
                    {
                        Name:       proto.String("GreetNoResponse"),
                        InputType:  proto.String(".connectrpc.greet.v1.GreetRequest"),
                        OutputType: proto.String(".connectrpc.greet.v1.GreetResponse"),
                        Options: &descriptorpb.MethodOptions{
                            IdempotencyLevel: descriptorpb.MethodOptions_NO_SIDE_EFFECTS.Enum(),
                        },
                    },
                },
            },
        },
    }

    root, err := parser.Parse([]*descriptorpb.FileDescriptorProto{fd})
    if err != nil {
        t.Fatalf("Parse failed: %v", err)
    }

    rpc := root.Services[0].RPCs[0]
    if rpc.HTTPMethod != "GET" {
        t.Errorf("expected HTTPMethod GET for NO_SIDE_EFFECTS, got %s", rpc.HTTPMethod)
    }
}

func TestParseMessage_FieldTypes(t *testing.T) {
    fd := &descriptorpb.FileDescriptorProto{
        Name:    proto.String("user/v1/user.proto"),
        Package: proto.String("user.v1"),
        MessageType: []*descriptorpb.DescriptorProto{
            {
                Name: proto.String("User"),
                Field: []*descriptorpb.FieldDescriptorProto{
                    {
                        Name:   proto.String("id"),
                        Number: proto.Int32(1),
                        Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
                        Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
                    },
                    {
                        Name:     proto.String("status"),
                        Number:   proto.Int32(2),
                        Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
                        TypeName: proto.String(".user.v1.UserStatus"),
                        Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
                    },
                    {
                        Name:     proto.String("tags"),
                        Number:   proto.Int32(3),
                        Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
                        Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
                    },
                },
            },
        },
    }

    root, err := parser.Parse([]*descriptorpb.FileDescriptorProto{fd})
    if err != nil {
        t.Fatalf("Parse failed: %v", err)
    }

    msg, ok := root.Messages[".user.v1.User"]
    if !ok {
        t.Fatal("User message not found in IR")
    }

    tests := []struct {
        index    int
        name     string
        typ      ir.FieldType
        label    ir.FieldLabel
        typeName string
    }{
        {0, "id", ir.FieldTypeString, ir.FieldLabelOptional, ""},
        {1, "status", ir.FieldTypeEnum, ir.FieldLabelOptional, ".user.v1.UserStatus"},
        {2, "tags", ir.FieldTypeString, ir.FieldLabelRepeated, ""},
    }

    for _, tt := range tests {
        f := msg.Fields[tt.index]
        if f.Name != tt.name {
            t.Errorf("[%d] expected name %s, got %s", tt.index, tt.name, f.Name)
        }
        if f.Type != tt.typ {
            t.Errorf("[%d] expected type %v, got %v", tt.index, tt.typ, f.Type)
        }
        if f.Label != tt.label {
            t.Errorf("[%d] expected label %v, got %v", tt.index, tt.label, f.Label)
        }
        if f.TypeName != tt.typeName {
            t.Errorf("[%d] expected typeName %s, got %s", tt.index, tt.typeName, f.TypeName)
        }
    }
}
```

### 12.4 Resolver ユニットテスト

```go
// internal/resolver/resolver_test.go

package resolver_test

import (
    "testing"

    "github.com/yourorg/connectview/internal/ir"
    "github.com/yourorg/connectview/internal/resolver"
)

// buildRoot はテスト用のIR Rootを構築するヘルパー
func buildRoot(services []*ir.Service, messages map[string]*ir.Message, enums map[string]*ir.Enum) *ir.Root {
    return &ir.Root{
        Services: services,
        Messages: messages,
        Enums:    enums,
    }
}

func TestResolve_BasicMessageRef(t *testing.T) {
    // Greet RPC の Request/Response が正しく解決されることを確認
    root := buildRoot(
        []*ir.Service{
            {
                Name: "GreetService",
                RPCs: []*ir.RPC{
                    {
                        Name: "Greet",
                        Request:  &ir.MessageRef{TypeName: ".connectrpc.greet.v1.GreetRequest"},
                        Response: &ir.MessageRef{TypeName: ".connectrpc.greet.v1.GreetResponse"},
                    },
                },
            },
        },
        map[string]*ir.Message{
            ".connectrpc.greet.v1.GreetRequest": {
                Name:     "GreetRequest",
                FullName: ".connectrpc.greet.v1.GreetRequest",
                Fields: []*ir.Field{
                    {Name: "name", Type: ir.FieldTypeString},
                },
            },
            ".connectrpc.greet.v1.GreetResponse": {
                Name:     "GreetResponse",
                FullName: ".connectrpc.greet.v1.GreetResponse",
                Fields: []*ir.Field{
                    {Name: "greeting", Type: ir.FieldTypeString},
                },
            },
        },
        nil,
    )

    r := resolver.New(root)
    if err := r.Resolve(); err != nil {
        t.Fatalf("Resolve failed: %v", err)
    }

    rpc := root.Services[0].RPCs[0]
    if rpc.Request.Resolved == nil {
        t.Error("Request.Resolved is nil after Resolve")
    }
    if rpc.Request.Resolved.Name != "GreetRequest" {
        t.Errorf("unexpected resolved name: %s", rpc.Request.Resolved.Name)
    }
    if rpc.Response.Resolved == nil {
        t.Error("Response.Resolved is nil after Resolve")
    }
}

func TestResolve_DirectRecursion(t *testing.T) {
    // TreeNode.children が再帰参照として検出されることを確認
    treeNode := &ir.Message{
        Name:     "TreeNode",
        FullName: ".tree.v1.TreeNode",
        Fields: []*ir.Field{
            {Name: "value", Type: ir.FieldTypeString},
            {Name: "children", Type: ir.FieldTypeMessage, TypeName: ".tree.v1.TreeNode", Label: ir.FieldLabelRepeated},
        },
    }

    root := buildRoot(
        []*ir.Service{
            {
                Name: "TreeService",
                RPCs: []*ir.RPC{
                    {
                        Name:     "GetTree",
                        Request:  &ir.MessageRef{TypeName: ".tree.v1.GetTreeRequest"},
                        Response: &ir.MessageRef{TypeName: ".tree.v1.GetTreeResponse"},
                    },
                },
            },
        },
        map[string]*ir.Message{
            ".tree.v1.GetTreeRequest": {
                FullName: ".tree.v1.GetTreeRequest",
                Fields:   []*ir.Field{{Name: "root_id", Type: ir.FieldTypeString}},
            },
            ".tree.v1.GetTreeResponse": {
                FullName: ".tree.v1.GetTreeResponse",
                Fields: []*ir.Field{
                    {Name: "root", Type: ir.FieldTypeMessage, TypeName: ".tree.v1.TreeNode"},
                },
            },
            ".tree.v1.TreeNode": treeNode,
        },
        nil,
    )

    r := resolver.New(root)
    if err := r.Resolve(); err != nil {
        t.Fatalf("Resolve failed: %v", err)
    }

    // children フィールドが IsRecursive = true になっているか確認
    childrenField := treeNode.Fields[1]
    if !childrenField.IsRecursive {
        t.Error("expected children to be marked as recursive")
    }
    // ResolvedMessage は型情報として設定されている（nilでない）
    if childrenField.ResolvedMessage == nil {
        t.Error("expected ResolvedMessage to be set even for recursive fields")
    }

    // value フィールドは再帰でない
    valueField := treeNode.Fields[0]
    if valueField.IsRecursive {
        t.Error("value field should not be marked as recursive")
    }
}

func TestResolve_MutualRecursion(t *testing.T) {
    // NodeA.b → NodeB.a → NodeA の相互再帰を検出
    nodeA := &ir.Message{
        Name:     "NodeA",
        FullName: ".mutual.v1.NodeA",
        Fields: []*ir.Field{
            {Name: "value", Type: ir.FieldTypeString},
            {Name: "b", Type: ir.FieldTypeMessage, TypeName: ".mutual.v1.NodeB"},
        },
    }
    nodeB := &ir.Message{
        Name:     "NodeB",
        FullName: ".mutual.v1.NodeB",
        Fields: []*ir.Field{
            {Name: "value", Type: ir.FieldTypeString},
            {Name: "a", Type: ir.FieldTypeMessage, TypeName: ".mutual.v1.NodeA"},
        },
    }

    root := buildRoot(
        []*ir.Service{
            {
                Name: "MutualService",
                RPCs: []*ir.RPC{
                    {
                        Name:     "Get",
                        Request:  &ir.MessageRef{TypeName: ".mutual.v1.GetRequest"},
                        Response: &ir.MessageRef{TypeName: ".mutual.v1.GetResponse"},
                    },
                },
            },
        },
        map[string]*ir.Message{
            ".mutual.v1.GetRequest": {
                FullName: ".mutual.v1.GetRequest",
                Fields: []*ir.Field{
                    {Name: "a", Type: ir.FieldTypeMessage, TypeName: ".mutual.v1.NodeA"},
                },
            },
            ".mutual.v1.GetResponse": {
                FullName: ".mutual.v1.GetResponse",
                Fields: []*ir.Field{
                    {Name: "result", Type: ir.FieldTypeMessage, TypeName: ".mutual.v1.NodeA"},
                },
            },
            ".mutual.v1.NodeA": nodeA,
            ".mutual.v1.NodeB": nodeB,
        },
        nil,
    )

    r := resolver.New(root)
    if err := r.Resolve(); err != nil {
        t.Fatalf("Resolve failed: %v", err)
    }

    // NodeA の b フィールドは展開される（パスに NodeA のみ）
    bField := nodeA.Fields[1]
    if bField.IsRecursive {
        t.Error("NodeA.b should not be recursive at this point")
    }

    // NodeB の a フィールドは再帰（パスに GetRequest → NodeA → NodeB が存在し、NodeAが再登場）
    aField := nodeB.Fields[1]
    if !aField.IsRecursive {
        t.Error("NodeB.a should be marked as recursive")
    }
}

func TestResolve_SiblingsSameType_NotRecursive(t *testing.T) {
    // 同じ型が兄弟フィールドに複数登場しても再帰ではない
    user := &ir.Message{
        Name:     "User",
        FullName: ".test.v1.User",
        Fields: []*ir.Field{
            {Name: "id", Type: ir.FieldTypeString},
        },
    }
    response := &ir.Message{
        Name:     "Response",
        FullName: ".test.v1.Response",
        Fields: []*ir.Field{
            {Name: "created_by", Type: ir.FieldTypeMessage, TypeName: ".test.v1.User"},
            {Name: "updated_by", Type: ir.FieldTypeMessage, TypeName: ".test.v1.User"},
        },
    }

    root := buildRoot(
        []*ir.Service{
            {
                RPCs: []*ir.RPC{
                    {
                        Request:  &ir.MessageRef{TypeName: ".test.v1.Response"},
                        Response: &ir.MessageRef{TypeName: ".test.v1.Response"},
                    },
                },
            },
        },
        map[string]*ir.Message{
            ".test.v1.User":     user,
            ".test.v1.Response": response,
        },
        nil,
    )

    r := resolver.New(root)
    if err := r.Resolve(); err != nil {
        t.Fatalf("Resolve failed: %v", err)
    }

    // 両フィールドとも IsRecursive = false
    for _, f := range response.Fields {
        if f.IsRecursive {
            t.Errorf("field %s should not be recursive", f.Name)
        }
        if f.ResolvedMessage == nil {
            t.Errorf("field %s should be resolved", f.Name)
        }
    }
}
```

### 12.5 Renderer ゴールデンファイルテスト

```go
// internal/renderer/renderer_test.go

package renderer_test

import (
    "flag"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "github.com/yourorg/connectview/internal/ir"
    "github.com/yourorg/connectview/internal/renderer"
)

// -update フラグでゴールデンファイルを更新する
var update = flag.Bool("update", false, "update golden files")

// greet サービスの IR を手動構築
func greetServiceIR() *ir.Root {
    return &ir.Root{
        Services: []*ir.Service{
            {
                Name:            "GreetService",
                FullName:         "connectrpc.greet.v1.GreetService",
                Comment:          "GreetService provides greeting functionality.",
                ConnectBasePath: "/connectrpc.greet.v1.GreetService/",
                RPCs: []*ir.RPC{
                    {
                        Name:        "Greet",
                        Comment:     "Greet sends a greeting to the named subject.",
                        ConnectPath: "/connectrpc.greet.v1.GreetService/Greet",
                        HTTPMethod:  "POST",
                        Request: &ir.MessageRef{
                            TypeName: ".connectrpc.greet.v1.GreetRequest",
                            Resolved: &ir.Message{
                                Name:    "GreetRequest",
                                Comment: "GreetRequest contains the subject to greet.",
                                Fields: []*ir.Field{
                                    {Name: "name", Type: ir.FieldTypeString, Label: ir.FieldLabelOptional, Comment: "The subject to greet."},
                                    {Name: "locale", Type: ir.FieldTypeString, Label: ir.FieldLabelOptional, Comment: "The locale for the greeting. Optional."},
                                },
                            },
                        },
                        Response: &ir.MessageRef{
                            TypeName: ".connectrpc.greet.v1.GreetResponse",
                            Resolved: &ir.Message{
                                Name:    "GreetResponse",
                                Comment: "GreetResponse contains the greeting message.",
                                Fields: []*ir.Field{
                                    {Name: "greeting", Type: ir.FieldTypeString, Label: ir.FieldLabelOptional, Comment: "The greeting."},
                                },
                            },
                        },
                    },
                },
            },
        },
    }
}

func TestRenderer_Golden(t *testing.T) {
    tests := []struct {
        name       string
        irBuilder  func() *ir.Root
        goldenFile string
    }{
        {
            name:       "greet_service",
            irBuilder:  greetServiceIR,
            goldenFile: "testdata/golden/greet_service.html",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            root := tt.irBuilder()
            r := renderer.New()
            got, err := r.Render(root)
            if err != nil {
                t.Fatalf("Render failed: %v", err)
            }

            if *update {
                // ゴールデンファイル更新モード
                dir := filepath.Dir(tt.goldenFile)
                os.MkdirAll(dir, 0755)
                os.WriteFile(tt.goldenFile, []byte(got), 0644)
                t.Logf("Updated golden file: %s", tt.goldenFile)
                return
            }

            // ゴールデンファイルと比較
            want, err := os.ReadFile(tt.goldenFile)
            if err != nil {
                t.Fatalf("Failed to read golden file %s: %v\n"+
                    "Run with -update flag to create it.", tt.goldenFile, err)
            }

            if got != string(want) {
                t.Errorf("HTML output differs from golden file.\n"+
                    "Run with -update flag to update.\n"+
                    "Diff (first difference):\n%s",
                    firstDiff(string(want), got))
            }
        })
    }
}

// RendererがサービスコメントをHTML内に含むか確認
func TestRenderer_ContainsServiceComment(t *testing.T) {
    root := greetServiceIR()
    r := renderer.New()
    html, err := r.Render(root)
    if err != nil {
        t.Fatalf("Render failed: %v", err)
    }

    expected := "GreetService provides greeting functionality."
    if !strings.Contains(html, expected) {
        t.Errorf("HTML does not contain service comment: %q", expected)
    }
}

// RendererがConnectRPCパスをHTML内に含むか確認
func TestRenderer_ContainsConnectPath(t *testing.T) {
    root := greetServiceIR()
    r := renderer.New()
    html, err := r.Render(root)
    if err != nil {
        t.Fatalf("Render failed: %v", err)
    }

    expected := "/connectrpc.greet.v1.GreetService/Greet"
    if !strings.Contains(html, expected) {
        t.Errorf("HTML does not contain connect path: %q", expected)
    }
}

// RendererがHTTPメソッドをHTML内に含むか確認
func TestRenderer_ContainsHTTPMethod(t *testing.T) {
    root := greetServiceIR()
    r := renderer.New()
    html, err := r.Render(root)
    if err != nil {
        t.Fatalf("Render failed: %v", err)
    }

    if !strings.Contains(html, "POST") {
        t.Error("HTML does not contain HTTP method POST")
    }
}

// RendererがスキーマJSONをHTML内に埋め込んでいるか確認
func TestRenderer_ContainsEmbeddedSchema(t *testing.T) {
    root := greetServiceIR()
    r := renderer.New()
    html, err := r.Render(root)
    if err != nil {
        t.Fatalf("Render failed: %v", err)
    }

    // JS側がschemaデータを参照するためのscriptタグが存在するか
    if !strings.Contains(html, "window.__CONNECTVIEW_SCHEMA__") {
        t.Error("HTML does not contain embedded schema JSON")
    }
    if !strings.Contains(html, "GreetService") {
        t.Error("HTML does not contain service name in embedded schema")
    }
}

// firstDiff は2つの文字列の最初の差分行を返す（テスト出力用ヘルパー）
func firstDiff(want, got string) string {
    wantLines := strings.Split(want, "\n")
    gotLines := strings.Split(got, "\n")
    for i := 0; i < len(wantLines) && i < len(gotLines); i++ {
        if wantLines[i] != gotLines[i] {
            return fmt.Sprintf("line %d:\nwant: %q\n got: %q", i+1, wantLines[i], gotLines[i])
        }
    }
    return "lengths differ"
}
```

### 12.6 期待出力のサンプル（IRのJSON表現）

```json
// testdata/expected/greet.ir.json
// Parser が greet.proto から生成するIRの期待値

{
  "services": [
    {
      "name": "GreetService",
      "fullName": "connectrpc.greet.v1.GreetService",
      "file": "greet/v1/greet.proto",
      "comment": "GreetService provides greeting functionality.",
      "connectBasePath": "/connectrpc.greet.v1.GreetService/",
      "rpcs": [
        {
          "name": "Greet",
          "comment": "Greet sends a greeting to the named subject.",
          "connectPath": "/connectrpc.greet.v1.GreetService/Greet",
          "httpMethod": "POST",
          "clientStreaming": false,
          "serverStreaming": false,
          "request": {
            "typeName": ".connectrpc.greet.v1.GreetRequest"
          },
          "response": {
            "typeName": ".connectrpc.greet.v1.GreetResponse"
          }
        },
        {
          "name": "GreetNoResponse",
          "comment": "GreetNoResponse is a one-way RPC with no side effects.",
          "connectPath": "/connectrpc.greet.v1.GreetService/GreetNoResponse",
          "httpMethod": "GET",
          "clientStreaming": false,
          "serverStreaming": false,
          "request": {
            "typeName": ".connectrpc.greet.v1.GreetRequest"
          },
          "response": {
            "typeName": ".connectrpc.greet.v1.GreetResponse"
          }
        }
      ]
    }
  ],
  "messages": {
    ".connectrpc.greet.v1.GreetRequest": {
      "name": "GreetRequest",
      "fullName": ".connectrpc.greet.v1.GreetRequest",
      "comment": "GreetRequest contains the subject to greet.",
      "fields": [
        {
          "name": "name",
          "number": 1,
          "type": 9,
          "label": 1,
          "comment": "The subject to greet."
        },
        {
          "name": "locale",
          "number": 2,
          "type": 9,
          "label": 1,
          "comment": "The locale for the greeting. Optional."
        }
      ]
    },
    ".connectrpc.greet.v1.GreetResponse": {
      "name": "GreetResponse",
      "fullName": ".connectrpc.greet.v1.GreetResponse",
      "comment": "GreetResponse contains the greeting message.",
      "fields": [
        {
          "name": "greeting",
          "number": 1,
          "type": 9,
          "label": 1,
          "comment": "The greeting."
        }
      ]
    }
  },
  "enums": {}
}
```

### 12.7 E2Eテスト（protocコマンド経由）

```go
// e2e/e2e_test.go

package e2e_test

import (
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "testing"
)

// TestE2E_GenerateHTML は実際にprotocコマンドを使って
// protoファイル → HTML生成までの全体フローを検証する
func TestE2E_GenerateHTML(t *testing.T) {
    // テスト環境にprotocがあるか確認
    if _, err := exec.LookPath("protoc"); err != nil {
        t.Skip("protoc not found in PATH, skipping E2E test")
    }

    // ビルド
    binaryPath := buildBinary(t)

    tmpDir := t.TempDir()
    outFile := filepath.Join(tmpDir, "index.html")

    // protoc実行
    cmd := exec.Command("protoc",
        "--plugin=protoc-gen-connectview="+binaryPath,
        "--connectview_out="+tmpDir,
        "-I", "testdata/proto",
        "testdata/proto/greet/v1/greet.proto",
    )
    out, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("protoc failed: %v\nOutput: %s", err, out)
    }

    // 出力ファイルが存在するか確認
    if _, err := os.Stat(outFile); os.IsNotExist(err) {
        t.Fatalf("expected output file %s was not created", outFile)
    }

    // HTMLの内容を検証
    content, err := os.ReadFile(outFile)
    if err != nil {
        t.Fatalf("failed to read output: %v", err)
    }
    html := string(content)

    checks := []struct {
        desc     string
        contains string
    }{
        {"service name", "GreetService"},
        {"rpc name", "Greet"},
        {"connect path", "/connectrpc.greet.v1.GreetService/Greet"},
        {"request message name", "GreetRequest"},
        {"response message name", "GreetResponse"},
        {"field name", "name"},
        {"service comment", "GreetService provides greeting functionality."},
        {"embedded schema JSON", "window.__CONNECTVIEW_SCHEMA__"},
        {"no external CDN", "cdn."},  // CDN参照がないこと（negativeチェック）
    }

    for _, c := range checks {
        if c.desc == "no external CDN" {
            if strings.Contains(html, c.contains) {
                t.Errorf("HTML should not contain external CDN reference: %q", c.contains)
            }
        } else {
            if !strings.Contains(html, c.contains) {
                t.Errorf("HTML does not contain expected %s: %q", c.desc, c.contains)
            }
        }
    }
}

func buildBinary(t *testing.T) string {
    t.Helper()
    tmpDir := t.TempDir()
    binaryPath := filepath.Join(tmpDir, "protoc-gen-connectview")
    cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/connectview")
    out, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("build failed: %v\nOutput: %s", err, out)
    }
    return binaryPath
}
```

### 12.8 buf.gen.yaml（testdata生成用・動作確認用）

```yaml
# buf.gen.yaml（connectview自身のtestdata確認用）
version: v2
plugins:
  - local: ["go", "run", "./cmd/connectview"]
    out: testdata/output
    opt:
      - base_url=http://localhost:8080
inputs:
  - directory: testdata/proto
```
