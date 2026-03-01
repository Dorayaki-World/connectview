# generateモード実装設計

## スコープ

`docs/design.md` のうち generateモード（protoc plugin として静的HTML生成）を実装する。
serveモードは後続で実装する。

## 設計書からの変更点

### 1. Map フィールドの IR サポート追加

設計書の `Field` 構造体に map 対応フィールドを追加:

```go
type Field struct {
    // ... 既存フィールド ...
    IsMap            bool
    MapKeyType       FieldType
    MapValueType     FieldType
    MapValueTypeName string // VALUE が MESSAGE/ENUM の場合の FQN
}
```

protobuf の map フィールドは内部的に synthetic `MapEntry` nested message として表現される。
Parser で `GetMapEntry()` を確認して map フィールドを識別する。

### 2. Optional フィールドの検出

proto3 の `optional` キーワードは内部的に synthetic oneof として表現される。
`FieldDescriptorProto.GetProto3Optional()` で判定し、`Field.IsOptional bool` を追加。
通常の oneof とは区別する。

### 3. コメント抽出

`SourceCodeInfo` のパス計算を正しく実装する。パスの構造:
- Service: [6, serviceIndex]
- Method: [6, serviceIndex, 2, methodIndex]
- Message: [4, messageIndex]
- Field: [4, messageIndex, 2, fieldIndex]
- Enum: [5, enumIndex]
- EnumValue: [5, enumIndex, 2, valueIndex]

## アーキテクチャ

```
.proto → protoc/buf → CodeGeneratorRequest(stdin)
  → plugin.Run()
    → parser.Parse(fileDescriptors) → ir.Root
    → resolver.New(root).Resolve() → 参照解決済みir.Root
    → renderer.New().Render(root) → HTML string
  → CodeGeneratorResponse(stdout) → index.html
```

## 実装コンポーネント

| パッケージ | ファイル | 責務 |
|-----------|---------|------|
| `internal/ir` | `ir.go` | IR 型定義 |
| `internal/parser` | `parser.go`, `comment.go` | FDP → IR 変換 |
| `internal/resolver` | `resolver.go` | 参照解決 + 再帰検出 |
| `internal/renderer` | `renderer.go`, `embed.go`, `assets/*` | IR → HTML |
| `internal/plugin` | `plugin.go` | protoc I/O |
| `cmd/connectview` | `main.go` | エントリポイント |

## テスト戦略

- Parser: 手動構築 FileDescriptorProto でユニットテスト
- Resolver: 基本参照、直接再帰、相互再帰、兄弟同型
- Renderer: コンテンツ存在チェック
- testdata/proto/: greet, user, tree, mutual の各 proto ファイル
