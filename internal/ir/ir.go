package ir

// Root は全体のIR。Rendererに渡す最上位の構造体
type Root struct {
	Files    []*File
	Services []*Service
	// FQN（"."始まり）→ Message のルックアップテーブル
	Messages map[string]*Message
	Enums    map[string]*Enum
}

type File struct {
	Name    string // "greet/v1/greet.proto"
	Package string // "connectrpc.greet.v1"
}

type Service struct {
	Name            string
	FullName        string
	File            string
	Comment         string
	RPCs            []*RPC
	ConnectBasePath string // "/{FullName}/"
}

type RPC struct {
	Name           string
	Comment        string
	ConnectPath    string // "/connectrpc.greet.v1.GreetService/Greet"
	HTTPMethod     string // "POST" or "GET"
	Request        *MessageRef
	Response       *MessageRef
	ClientStreaming bool
	ServerStreaming bool
}

type MessageRef struct {
	TypeName string   // FQN（"."始まり）
	Resolved *Message // Resolverが設定
}

type Message struct {
	Name           string
	FullName       string
	Comment        string
	Fields         []*Field
	NestedMessages []*Message
	NestedEnums    []*Enum
}

type Field struct {
	Name             string
	Number           int32
	Type             FieldType
	TypeName         string // MESSAGE/ENUMの場合のFQN（"."始まり）
	Label            FieldLabel
	Comment          string
	OneofName        string
	IsOptional       bool // proto3 optional
	IsMap            bool
	MapKeyType       FieldType
	MapValueType     FieldType
	MapValueTypeName string // VALUE が MESSAGE/ENUM の場合のFQN
	ResolvedMessage  *Message
	ResolvedEnum     *Enum
	IsRecursive      bool
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
	FieldTypeMessage  FieldType = 11
	FieldTypeBytes    FieldType = 12
	FieldTypeUint32   FieldType = 13
	FieldTypeEnum     FieldType = 14
	FieldTypeSfixed32 FieldType = 15
	FieldTypeSfixed64 FieldType = 16
	FieldTypeSint32   FieldType = 17
	FieldTypeSint64   FieldType = 18
)

type FieldLabel int32

const (
	FieldLabelOptional FieldLabel = 1
	FieldLabelRequired FieldLabel = 2
	FieldLabelRepeated FieldLabel = 3
)

type Enum struct {
	Name     string
	FullName string
	Comment  string
	Values   []*EnumValue
}

type EnumValue struct {
	Name    string
	Number  int32
	Comment string
}
