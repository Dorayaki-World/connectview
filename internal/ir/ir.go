// internal/ir/ir.go
package ir

// Root is the top-level IR structure passed to the renderer.
type Root struct {
	Files    []*File
	Services []*Service
	Messages map[string]*Message // FQN ("." prefix) → Message lookup
	Enums    map[string]*Enum    // FQN ("." prefix) → Enum lookup
}

type File struct {
	Name    string // "greet/v1/greet.proto"
	Package string // "connectrpc.greet.v1"
}

type Service struct {
	Name            string // "GreetService"
	FullName        string // "connectrpc.greet.v1.GreetService"
	File            string // "greet/v1/greet.proto"
	Comment         string
	RPCs            []*RPC
	ConnectBasePath string // "/connectrpc.greet.v1.GreetService/"
}

type RPC struct {
	Name           string // "Greet"
	Comment        string
	ConnectPath    string // "/connectrpc.greet.v1.GreetService/Greet"
	HTTPMethod     string // "POST" or "GET"
	Request        *MessageRef
	Response       *MessageRef
	ClientStreaming bool
	ServerStreaming bool
}

type MessageRef struct {
	TypeName string   // FQN e.g. ".connectrpc.greet.v1.GreetRequest"
	Resolved *Message // populated by resolver
}

type Message struct {
	Name           string // "GreetRequest"
	FullName       string // ".connectrpc.greet.v1.GreetRequest"
	Comment        string
	Fields         []*Field
	NestedMessages []*Message
	NestedEnums    []*Enum
	IsMapEntry     bool // synthetic map entry message
}

type Field struct {
	Name             string
	Number           int32
	Type             FieldType
	TypeName         string // FQN for MESSAGE/ENUM types
	Label            FieldLabel
	Comment          string
	IsOptional       bool   // proto3 optional keyword
	OneofName        string // non-empty if part of a real oneof
	IsMap            bool
	MapKeyType       FieldType
	MapValueType     FieldType
	MapValueTypeName string // FQN if map value is MESSAGE/ENUM
	ResolvedMessage  *Message
	ResolvedEnum     *Enum
	IsRecursive      bool // set by resolver
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

// String returns the human-readable name of the field type.
func (ft FieldType) String() string {
	switch ft {
	case FieldTypeDouble:
		return "double"
	case FieldTypeFloat:
		return "float"
	case FieldTypeInt64:
		return "int64"
	case FieldTypeUint64:
		return "uint64"
	case FieldTypeInt32:
		return "int32"
	case FieldTypeFixed64:
		return "fixed64"
	case FieldTypeFixed32:
		return "fixed32"
	case FieldTypeBool:
		return "bool"
	case FieldTypeString:
		return "string"
	case FieldTypeMessage:
		return "message"
	case FieldTypeBytes:
		return "bytes"
	case FieldTypeUint32:
		return "uint32"
	case FieldTypeEnum:
		return "enum"
	case FieldTypeSfixed32:
		return "sfixed32"
	case FieldTypeSfixed64:
		return "sfixed64"
	case FieldTypeSint32:
		return "sint32"
	case FieldTypeSint64:
		return "sint64"
	default:
		return "unknown"
	}
}

type FieldLabel int32

const (
	FieldLabelOptional FieldLabel = 1
	FieldLabelRequired FieldLabel = 2
	FieldLabelRepeated FieldLabel = 3
)

type Enum struct {
	Name     string // "UserStatus"
	FullName string // ".user.v1.UserStatus"
	Comment  string
	Values   []*EnumValue
}

type EnumValue struct {
	Name    string // "USER_STATUS_ACTIVE"
	Number  int32  // 1
	Comment string
}
