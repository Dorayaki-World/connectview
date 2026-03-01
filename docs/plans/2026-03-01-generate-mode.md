# generateモード Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement `protoc-gen-connectview` — a protoc plugin that generates an interactive single-file HTML viewer for ConnectRPC services from .proto files.

**Architecture:** Uses `google.golang.org/protobuf/compiler/protogen` (not raw pluginpb) for reading CodeGeneratorRequest, which gives us free comment extraction, type resolution, map detection, and proto3 optional detection. The flow is: protogen.Files → IR → Resolver (recursion detection) → Renderer (HTML with embedded schema JSON + inline CSS/JS). This is a significant improvement over the design doc's raw descriptorpb approach.

**Tech Stack:** Go 1.26, `google.golang.org/protobuf` (protogen + protoreflect), `html/template`, `go:embed`

**Design doc changes:** The design doc specifies `internal/plugin/plugin.go` with manual stdin/stdout handling and `internal/parser/comment.go` for SourceCodeInfo parsing. With protogen, both are unnecessary — protogen handles CodeGeneratorRequest I/O, comment extraction, type resolution, map detection, and optional field detection automatically. We remove `internal/plugin/` and `internal/parser/comment.go` from the architecture. The parser now accepts `[]*protogen.File` instead of `[]*descriptorpb.FileDescriptorProto`.

---

### Task 1: Project Setup and Dependencies

**Files:**
- Modify: `go.mod`
- Create: `cmd/connectview/main.go` (stub)

**Step 1: Add protobuf dependency**

```bash
cd /Users/abe/git/oss/connectview
go get google.golang.org/protobuf
```

**Step 2: Create stub main.go**

```go
// cmd/connectview/main.go
package main

func main() {
	// TODO: implement protoc plugin
}
```

**Step 3: Verify build**

```bash
go build ./cmd/connectview
```
Expected: SUCCESS (no errors)

**Step 4: Commit**

```bash
git add go.mod go.sum cmd/
git commit -m "chore: add protobuf dependency and stub entry point"
```

---

### Task 2: Define IR Types

**Files:**
- Create: `internal/ir/ir.go`

**Step 1: Create IR type definitions**

```go
// internal/ir/ir.go
package ir

// Root is the top-level IR structure passed to the renderer.
type Root struct {
	Files    []*File
	Services []*Service
	Messages map[string]*Message // FQN → Message lookup
	Enums    map[string]*Enum    // FQN → Enum lookup
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
	Name            string // "Greet"
	Comment         string
	ConnectPath     string // "/connectrpc.greet.v1.GreetService/Greet"
	HTTPMethod      string // "POST" or "GET"
	Request         *MessageRef
	Response        *MessageRef
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

// FieldTypeName returns the human-readable name of the type.
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
```

**Step 2: Verify compilation**

```bash
go build ./internal/ir/
```
Expected: SUCCESS

**Step 3: Commit**

```bash
git add internal/ir/
git commit -m "feat: define IR types for proto schema representation"
```

---

### Task 3: Implement Parser

The parser converts `[]*protogen.File` to `*ir.Root`. Uses protogen's built-in comment extraction, map detection, and optional detection.

**Files:**
- Create: `internal/parser/parser.go`
- Create: `internal/parser/parser_test.go`

**Step 1: Write parser tests**

The tests use `protogen.Options{}.New()` with hand-built `CodeGeneratorRequest` to create protogen objects for testing.

```go
// internal/parser/parser_test.go
package parser_test

import (
	"testing"

	"github.com/Dorayaki-World/connectview/internal/ir"
	"github.com/Dorayaki-World/connectview/internal/parser"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	descriptorpb "google.golang.org/protobuf/types/descriptorpb"
	pluginpb "google.golang.org/protobuf/types/pluginpb"
)

// newPlugin creates a protogen.Plugin from hand-built file descriptors for testing.
func newPlugin(t *testing.T, files []*descriptorpb.FileDescriptorProto, filesToGenerate []string) *protogen.Plugin {
	t.Helper()
	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: filesToGenerate,
		ProtoFile:      files,
	}
	plugin, err := protogen.Options{}.New(req)
	if err != nil {
		t.Fatalf("protogen.Options{}.New() failed: %v", err)
	}
	return plugin
}

func TestParseService_Basic(t *testing.T) {
	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("greet/v1/greet.proto"),
		Package: proto.String("connectrpc.greet.v1"),
		Syntax:  proto.String("proto3"),
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
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("GreetRequest"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("name"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
			},
			{
				Name: proto.String("GreetResponse"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("greeting"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
			},
		},
	}

	plugin := newPlugin(t, []*descriptorpb.FileDescriptorProto{fd}, []string{"greet/v1/greet.proto"})
	root := parser.Parse(plugin)

	if len(root.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(root.Services))
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
		t.Errorf("expected POST, got %s", rpc.HTTPMethod)
	}
	if rpc.Request.TypeName != ".connectrpc.greet.v1.GreetRequest" {
		t.Errorf("unexpected request typeName: %s", rpc.Request.TypeName)
	}
}

func TestParseRPC_IdempotencyLevel_GET(t *testing.T) {
	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("greet/v1/greet.proto"),
		Package: proto.String("connectrpc.greet.v1"),
		Syntax:  proto.String("proto3"),
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: proto.String("GreetService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       proto.String("GreetNoSideEffects"),
						InputType:  proto.String(".connectrpc.greet.v1.GreetRequest"),
						OutputType: proto.String(".connectrpc.greet.v1.GreetResponse"),
						Options: &descriptorpb.MethodOptions{
							IdempotencyLevel: descriptorpb.MethodOptions_NO_SIDE_EFFECTS.Enum(),
						},
					},
				},
			},
		},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("GreetRequest")},
			{Name: proto.String("GreetResponse")},
		},
	}

	plugin := newPlugin(t, []*descriptorpb.FileDescriptorProto{fd}, []string{"greet/v1/greet.proto"})
	root := parser.Parse(plugin)

	rpc := root.Services[0].RPCs[0]
	if rpc.HTTPMethod != "GET" {
		t.Errorf("expected GET for NO_SIDE_EFFECTS, got %s", rpc.HTTPMethod)
	}
}

func TestParseMessage_FieldTypes(t *testing.T) {
	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("user/v1/user.proto"),
		Package: proto.String("user.v1"),
		Syntax:  proto.String("proto3"),
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
						Name:   proto.String("tags"),
						Number: proto.Int32(3),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
				},
			},
		},
		EnumType: []*descriptorpb.EnumDescriptorProto{
			{
				Name: proto.String("UserStatus"),
				Value: []*descriptorpb.EnumValueDescriptorProto{
					{Name: proto.String("USER_STATUS_UNSPECIFIED"), Number: proto.Int32(0)},
					{Name: proto.String("USER_STATUS_ACTIVE"), Number: proto.Int32(1)},
				},
			},
		},
	}

	plugin := newPlugin(t, []*descriptorpb.FileDescriptorProto{fd}, []string{"user/v1/user.proto"})
	root := parser.Parse(plugin)

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

	// Verify enum was parsed
	enum, ok := root.Enums[".user.v1.UserStatus"]
	if !ok {
		t.Fatal("UserStatus enum not found in IR")
	}
	if len(enum.Values) != 2 {
		t.Errorf("expected 2 enum values, got %d", len(enum.Values))
	}
}

func TestParseMessage_Streaming(t *testing.T) {
	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("stream/v1/stream.proto"),
		Package: proto.String("stream.v1"),
		Syntax:  proto.String("proto3"),
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: proto.String("StreamService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:            proto.String("ServerStream"),
						InputType:       proto.String(".stream.v1.Req"),
						OutputType:      proto.String(".stream.v1.Resp"),
						ServerStreaming: proto.Bool(true),
					},
				},
			},
		},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("Req")},
			{Name: proto.String("Resp")},
		},
	}

	plugin := newPlugin(t, []*descriptorpb.FileDescriptorProto{fd}, []string{"stream/v1/stream.proto"})
	root := parser.Parse(plugin)

	rpc := root.Services[0].RPCs[0]
	if !rpc.ServerStreaming {
		t.Error("expected ServerStreaming to be true")
	}
	if rpc.ClientStreaming {
		t.Error("expected ClientStreaming to be false")
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/parser/ -v
```
Expected: FAIL (package doesn't exist yet)

**Step 3: Implement parser**

```go
// internal/parser/parser.go
package parser

import (
	"strings"

	"github.com/Dorayaki-World/connectview/internal/ir"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Parse converts protogen files into the IR representation.
func Parse(plugin *protogen.Plugin) *ir.Root {
	root := &ir.Root{
		Messages: make(map[string]*ir.Message),
		Enums:    make(map[string]*ir.Enum),
	}

	for _, f := range plugin.Files {
		if !f.Generate {
			continue
		}

		root.Files = append(root.Files, &ir.File{
			Name:    f.Desc.Path(),
			Package: string(f.Desc.Package()),
		})

		// Parse messages (recursive for nested)
		for _, msg := range f.Messages {
			parseMessage(root, msg)
		}

		// Parse top-level enums
		for _, enum := range f.Enums {
			e := parseEnum(enum)
			root.Enums[e.FullName] = e
		}

		// Parse services
		for _, svc := range f.Services {
			root.Services = append(root.Services, parseService(svc))
		}
	}

	return root
}

func parseService(svc *protogen.Service) *ir.Service {
	fullName := string(svc.Desc.FullName())
	s := &ir.Service{
		Name:            string(svc.Desc.Name()),
		FullName:        fullName,
		File:            svc.Desc.ParentFile().Path(),
		Comment:         cleanComment(string(svc.Comments.Leading)),
		ConnectBasePath: "/" + fullName + "/",
	}

	for _, method := range svc.Methods {
		s.RPCs = append(s.RPCs, parseRPC(method, fullName))
	}

	return s
}

func parseRPC(method *protogen.Method, serviceFQN string) *ir.RPC {
	httpMethod := "POST"
	if method.Desc.Options() != nil {
		opts, ok := method.Desc.Options().(*descriptorpb.MethodOptions)
		if ok && opts.GetIdempotencyLevel() == descriptorpb.MethodOptions_NO_SIDE_EFFECTS {
			httpMethod = "GET"
		}
	}

	// Actually use protoreflect for idempotency check
	// method.Desc.Options() returns protoreflect.ProtoMessage which we need to handle differently

	return &ir.RPC{
		Name:            string(method.Desc.Name()),
		Comment:         cleanComment(string(method.Comments.Leading)),
		ConnectPath:     "/" + serviceFQN + "/" + string(method.Desc.Name()),
		HTTPMethod:      httpMethod,
		Request:         &ir.MessageRef{TypeName: "." + string(method.Desc.Input().FullName())},
		Response:        &ir.MessageRef{TypeName: "." + string(method.Desc.Output().FullName())},
		ClientStreaming: method.Desc.IsStreamingClient(),
		ServerStreaming: method.Desc.IsStreamingServer(),
	}
}

func parseMessage(root *ir.Root, msg *protogen.Message) {
	if msg.Desc.IsMapEntry() {
		return // skip synthetic map entry messages
	}

	fqn := "." + string(msg.Desc.FullName())
	m := &ir.Message{
		Name:     string(msg.Desc.Name()),
		FullName: fqn,
		Comment:  cleanComment(string(msg.Comments.Leading)),
	}

	// Parse fields
	for _, field := range msg.Fields {
		m.Fields = append(m.Fields, parseField(field))
	}

	// Parse nested messages
	for _, nested := range msg.Messages {
		parseMessage(root, nested)
		if !nested.Desc.IsMapEntry() {
			nestedFQN := "." + string(nested.Desc.FullName())
			if nm, ok := root.Messages[nestedFQN]; ok {
				m.NestedMessages = append(m.NestedMessages, nm)
			}
		}
	}

	// Parse nested enums
	for _, enum := range msg.Enums {
		e := parseEnum(enum)
		root.Enums[e.FullName] = e
		m.NestedEnums = append(m.NestedEnums, e)
	}

	root.Messages[fqn] = m
}

func parseField(field *protogen.Field) *ir.Field {
	f := &ir.Field{
		Name:       string(field.Desc.Name()),
		Number:     int32(field.Desc.Number()),
		Type:       kindToFieldType(field.Desc.Kind()),
		Label:      cardinalityToLabel(field.Desc.Cardinality()),
		Comment:    cleanComment(string(field.Comments.Leading)),
		IsOptional: field.Desc.HasOptionalKeyword(),
	}

	// Handle oneof (skip synthetic oneofs for proto3 optional)
	if field.Oneof != nil && !field.Oneof.Desc.IsSynthetic() {
		f.OneofName = string(field.Oneof.Desc.Name())
	}

	// Handle map fields
	if field.Desc.IsMap() {
		f.IsMap = true
		f.Label = ir.FieldLabelRepeated // maps are repeated internally
		mapEntry := field.Message
		keyField := mapEntry.Fields[0]
		valueField := mapEntry.Fields[1]
		f.MapKeyType = kindToFieldType(keyField.Desc.Kind())
		f.MapValueType = kindToFieldType(valueField.Desc.Kind())
		if valueField.Desc.Kind() == protoreflect.MessageKind {
			f.MapValueTypeName = "." + string(valueField.Desc.Message().FullName())
		} else if valueField.Desc.Kind() == protoreflect.EnumKind {
			f.MapValueTypeName = "." + string(valueField.Desc.Enum().FullName())
		}
		f.Type = ir.FieldTypeMessage // map fields have message type (the MapEntry)
		f.TypeName = "" // maps don't need TypeName
		return f
	}

	// Handle message/enum type references
	if field.Desc.Kind() == protoreflect.MessageKind {
		f.TypeName = "." + string(field.Desc.Message().FullName())
	} else if field.Desc.Kind() == protoreflect.EnumKind {
		f.TypeName = "." + string(field.Desc.Enum().FullName())
	}

	return f
}

func parseEnum(enum *protogen.Enum) *ir.Enum {
	fqn := "." + string(enum.Desc.FullName())
	e := &ir.Enum{
		Name:     string(enum.Desc.Name()),
		FullName: fqn,
		Comment:  cleanComment(string(enum.Comments.Leading)),
	}

	for _, val := range enum.Values {
		e.Values = append(e.Values, &ir.EnumValue{
			Name:    string(val.Desc.Name()),
			Number:  int32(val.Desc.Number()),
			Comment: cleanComment(string(val.Comments.Leading)),
		})
	}

	return e
}

func kindToFieldType(kind protoreflect.Kind) ir.FieldType {
	switch kind {
	case protoreflect.DoubleKind:
		return ir.FieldTypeDouble
	case protoreflect.FloatKind:
		return ir.FieldTypeFloat
	case protoreflect.Int64Kind:
		return ir.FieldTypeInt64
	case protoreflect.Uint64Kind:
		return ir.FieldTypeUint64
	case protoreflect.Int32Kind:
		return ir.FieldTypeInt32
	case protoreflect.Fixed64Kind:
		return ir.FieldTypeFixed64
	case protoreflect.Fixed32Kind:
		return ir.FieldTypeFixed32
	case protoreflect.BoolKind:
		return ir.FieldTypeBool
	case protoreflect.StringKind:
		return ir.FieldTypeString
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return ir.FieldTypeMessage
	case protoreflect.BytesKind:
		return ir.FieldTypeBytes
	case protoreflect.Uint32Kind:
		return ir.FieldTypeUint32
	case protoreflect.EnumKind:
		return ir.FieldTypeEnum
	case protoreflect.Sfixed32Kind:
		return ir.FieldTypeSfixed32
	case protoreflect.Sfixed64Kind:
		return ir.FieldTypeSfixed64
	case protoreflect.Sint32Kind:
		return ir.FieldTypeSint32
	case protoreflect.Sint64Kind:
		return ir.FieldTypeSint64
	default:
		return ir.FieldTypeString
	}
}

func cardinalityToLabel(c protoreflect.Cardinality) ir.FieldLabel {
	switch c {
	case protoreflect.Repeated:
		return ir.FieldLabelRepeated
	case protoreflect.Required:
		return ir.FieldLabelRequired
	default:
		return ir.FieldLabelOptional
	}
}

func cleanComment(s string) string {
	s = strings.TrimSpace(s)
	return s
}
```

Note: The idempotency level check needs to use protoreflect properly. The actual implementation should use:

```go
import "google.golang.org/protobuf/types/descriptorpb"

func getHTTPMethod(method *protogen.Method) string {
	opts := method.Desc.Options()
	if opts != nil {
		methodOpts, ok := opts.(*descriptorpb.MethodOptions)
		if ok && methodOpts.GetIdempotencyLevel() == descriptorpb.MethodOptions_NO_SIDE_EFFECTS {
			return "GET"
		}
	}
	return "POST"
}
```

**Step 4: Run tests**

```bash
go test ./internal/parser/ -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/parser/
git commit -m "feat: implement parser to convert protogen files to IR"
```

---

### Task 4: Implement Resolver

The resolver resolves cross-file message/enum references and detects recursive references using the visitedInPath algorithm from the design doc.

**Files:**
- Create: `internal/resolver/resolver.go`
- Create: `internal/resolver/resolver_test.go`

**Step 1: Write resolver tests**

```go
// internal/resolver/resolver_test.go
package resolver_test

import (
	"testing"

	"github.com/Dorayaki-World/connectview/internal/ir"
	"github.com/Dorayaki-World/connectview/internal/resolver"
)

func TestResolve_BasicMessageRef(t *testing.T) {
	root := &ir.Root{
		Services: []*ir.Service{
			{
				Name: "GreetService",
				RPCs: []*ir.RPC{
					{
						Name:     "Greet",
						Request:  &ir.MessageRef{TypeName: ".greet.v1.GreetRequest"},
						Response: &ir.MessageRef{TypeName: ".greet.v1.GreetResponse"},
					},
				},
			},
		},
		Messages: map[string]*ir.Message{
			".greet.v1.GreetRequest": {
				Name:     "GreetRequest",
				FullName: ".greet.v1.GreetRequest",
				Fields:   []*ir.Field{{Name: "name", Type: ir.FieldTypeString}},
			},
			".greet.v1.GreetResponse": {
				Name:     "GreetResponse",
				FullName: ".greet.v1.GreetResponse",
				Fields:   []*ir.Field{{Name: "greeting", Type: ir.FieldTypeString}},
			},
		},
		Enums: map[string]*ir.Enum{},
	}

	r := resolver.New(root)
	if err := r.Resolve(); err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	rpc := root.Services[0].RPCs[0]
	if rpc.Request.Resolved == nil {
		t.Error("Request.Resolved is nil")
	}
	if rpc.Request.Resolved.Name != "GreetRequest" {
		t.Errorf("unexpected resolved name: %s", rpc.Request.Resolved.Name)
	}
	if rpc.Response.Resolved == nil {
		t.Error("Response.Resolved is nil")
	}
}

func TestResolve_NestedMessageField(t *testing.T) {
	root := &ir.Root{
		Services: []*ir.Service{
			{
				RPCs: []*ir.RPC{
					{
						Request:  &ir.MessageRef{TypeName: ".test.v1.Outer"},
						Response: &ir.MessageRef{TypeName: ".test.v1.Outer"},
					},
				},
			},
		},
		Messages: map[string]*ir.Message{
			".test.v1.Outer": {
				FullName: ".test.v1.Outer",
				Fields: []*ir.Field{
					{Name: "inner", Type: ir.FieldTypeMessage, TypeName: ".test.v1.Inner"},
				},
			},
			".test.v1.Inner": {
				FullName: ".test.v1.Inner",
				Fields:   []*ir.Field{{Name: "value", Type: ir.FieldTypeString}},
			},
		},
		Enums: map[string]*ir.Enum{},
	}

	r := resolver.New(root)
	if err := r.Resolve(); err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	outerField := root.Messages[".test.v1.Outer"].Fields[0]
	if outerField.ResolvedMessage == nil {
		t.Error("inner field ResolvedMessage is nil")
	}
	if outerField.IsRecursive {
		t.Error("inner field should not be recursive")
	}
}

func TestResolve_EnumField(t *testing.T) {
	root := &ir.Root{
		Services: []*ir.Service{
			{
				RPCs: []*ir.RPC{
					{
						Request:  &ir.MessageRef{TypeName: ".test.v1.Req"},
						Response: &ir.MessageRef{TypeName: ".test.v1.Req"},
					},
				},
			},
		},
		Messages: map[string]*ir.Message{
			".test.v1.Req": {
				FullName: ".test.v1.Req",
				Fields: []*ir.Field{
					{Name: "status", Type: ir.FieldTypeEnum, TypeName: ".test.v1.Status"},
				},
			},
		},
		Enums: map[string]*ir.Enum{
			".test.v1.Status": {
				FullName: ".test.v1.Status",
				Values:   []*ir.EnumValue{{Name: "UNSPECIFIED", Number: 0}},
			},
		},
	}

	r := resolver.New(root)
	if err := r.Resolve(); err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	field := root.Messages[".test.v1.Req"].Fields[0]
	if field.ResolvedEnum == nil {
		t.Error("status field ResolvedEnum is nil")
	}
}

func TestResolve_DirectRecursion(t *testing.T) {
	treeNode := &ir.Message{
		Name:     "TreeNode",
		FullName: ".tree.v1.TreeNode",
		Fields: []*ir.Field{
			{Name: "value", Type: ir.FieldTypeString},
			{Name: "children", Type: ir.FieldTypeMessage, TypeName: ".tree.v1.TreeNode", Label: ir.FieldLabelRepeated},
		},
	}

	root := &ir.Root{
		Services: []*ir.Service{
			{
				RPCs: []*ir.RPC{
					{
						Request:  &ir.MessageRef{TypeName: ".tree.v1.GetTreeRequest"},
						Response: &ir.MessageRef{TypeName: ".tree.v1.GetTreeResponse"},
					},
				},
			},
		},
		Messages: map[string]*ir.Message{
			".tree.v1.GetTreeRequest":  {FullName: ".tree.v1.GetTreeRequest", Fields: []*ir.Field{{Name: "root_id", Type: ir.FieldTypeString}}},
			".tree.v1.GetTreeResponse": {FullName: ".tree.v1.GetTreeResponse", Fields: []*ir.Field{{Name: "root", Type: ir.FieldTypeMessage, TypeName: ".tree.v1.TreeNode"}}},
			".tree.v1.TreeNode":        treeNode,
		},
		Enums: map[string]*ir.Enum{},
	}

	r := resolver.New(root)
	if err := r.Resolve(); err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	childrenField := treeNode.Fields[1]
	if !childrenField.IsRecursive {
		t.Error("expected children to be marked as recursive")
	}
	if childrenField.ResolvedMessage == nil {
		t.Error("expected ResolvedMessage to be set even for recursive fields")
	}

	valueField := treeNode.Fields[0]
	if valueField.IsRecursive {
		t.Error("value field should not be recursive")
	}
}

func TestResolve_MutualRecursion(t *testing.T) {
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

	root := &ir.Root{
		Services: []*ir.Service{
			{
				RPCs: []*ir.RPC{
					{
						Request:  &ir.MessageRef{TypeName: ".mutual.v1.GetRequest"},
						Response: &ir.MessageRef{TypeName: ".mutual.v1.GetResponse"},
					},
				},
			},
		},
		Messages: map[string]*ir.Message{
			".mutual.v1.GetRequest":  {FullName: ".mutual.v1.GetRequest", Fields: []*ir.Field{{Name: "a", Type: ir.FieldTypeMessage, TypeName: ".mutual.v1.NodeA"}}},
			".mutual.v1.GetResponse": {FullName: ".mutual.v1.GetResponse", Fields: []*ir.Field{{Name: "result", Type: ir.FieldTypeMessage, TypeName: ".mutual.v1.NodeA"}}},
			".mutual.v1.NodeA":       nodeA,
			".mutual.v1.NodeB":       nodeB,
		},
		Enums: map[string]*ir.Enum{},
	}

	r := resolver.New(root)
	if err := r.Resolve(); err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	bField := nodeA.Fields[1]
	if bField.IsRecursive {
		t.Error("NodeA.b should not be recursive at this point")
	}

	aField := nodeB.Fields[1]
	if !aField.IsRecursive {
		t.Error("NodeB.a should be marked as recursive")
	}
}

func TestResolve_SiblingsSameType_NotRecursive(t *testing.T) {
	user := &ir.Message{
		FullName: ".test.v1.User",
		Fields:   []*ir.Field{{Name: "id", Type: ir.FieldTypeString}},
	}
	response := &ir.Message{
		FullName: ".test.v1.Response",
		Fields: []*ir.Field{
			{Name: "created_by", Type: ir.FieldTypeMessage, TypeName: ".test.v1.User"},
			{Name: "updated_by", Type: ir.FieldTypeMessage, TypeName: ".test.v1.User"},
		},
	}

	root := &ir.Root{
		Services: []*ir.Service{
			{
				RPCs: []*ir.RPC{
					{
						Request:  &ir.MessageRef{TypeName: ".test.v1.Response"},
						Response: &ir.MessageRef{TypeName: ".test.v1.Response"},
					},
				},
			},
		},
		Messages: map[string]*ir.Message{
			".test.v1.User":     user,
			".test.v1.Response": response,
		},
		Enums: map[string]*ir.Enum{},
	}

	r := resolver.New(root)
	if err := r.Resolve(); err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

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

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/resolver/ -v
```
Expected: FAIL

**Step 3: Implement resolver**

```go
// internal/resolver/resolver.go
package resolver

import (
	"fmt"

	"github.com/Dorayaki-World/connectview/internal/ir"
)

type Resolver struct {
	root *ir.Root
}

func New(root *ir.Root) *Resolver {
	return &Resolver{root: root}
}

func (r *Resolver) Resolve() error {
	for _, svc := range r.root.Services {
		for _, rpc := range svc.RPCs {
			if err := r.resolveMessageRef(rpc.Request, nil); err != nil {
				return fmt.Errorf("resolve request for %s: %w", rpc.Name, err)
			}
			if err := r.resolveMessageRef(rpc.Response, nil); err != nil {
				return fmt.Errorf("resolve response for %s: %w", rpc.Name, err)
			}
		}
	}
	return nil
}

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
		if field.Type == ir.FieldTypeMessage && field.TypeName != "" && !field.IsMap {
			if visitedInPath[field.TypeName] {
				field.IsRecursive = true
				field.ResolvedMessage = r.root.Messages[field.TypeName]
				continue
			}
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

func copyMap(m map[string]bool) map[string]bool {
	cp := make(map[string]bool, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
```

**Step 4: Run tests**

```bash
go test ./internal/resolver/ -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/resolver/
git commit -m "feat: implement resolver for cross-file refs and recursion detection"
```

---

### Task 5: Implement Renderer and Assets

The renderer converts the resolved IR into a single self-contained HTML file with embedded CSS, JS, and schema JSON.

**Files:**
- Create: `internal/renderer/renderer.go`
- Create: `internal/renderer/embed.go`
- Create: `internal/renderer/assets/index.html.tmpl`
- Create: `internal/renderer/assets/style.css`
- Create: `internal/renderer/assets/app.js`
- Create: `internal/renderer/renderer_test.go`

**Step 1: Write renderer tests**

```go
// internal/renderer/renderer_test.go
package renderer_test

import (
	"strings"
	"testing"

	"github.com/Dorayaki-World/connectview/internal/ir"
	"github.com/Dorayaki-World/connectview/internal/renderer"
)

func greetServiceIR() *ir.Root {
	return &ir.Root{
		Services: []*ir.Service{
			{
				Name:            "GreetService",
				FullName:        "connectrpc.greet.v1.GreetService",
				Comment:         "GreetService provides greeting functionality.",
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
									{Name: "name", Type: ir.FieldTypeString, Number: 1, Label: ir.FieldLabelOptional, Comment: "The subject to greet."},
									{Name: "locale", Type: ir.FieldTypeString, Number: 2, Label: ir.FieldLabelOptional, Comment: "The locale for the greeting. Optional.", IsOptional: true},
								},
							},
						},
						Response: &ir.MessageRef{
							TypeName: ".connectrpc.greet.v1.GreetResponse",
							Resolved: &ir.Message{
								Name:    "GreetResponse",
								Comment: "GreetResponse contains the greeting message.",
								Fields: []*ir.Field{
									{Name: "greeting", Type: ir.FieldTypeString, Number: 1, Label: ir.FieldLabelOptional, Comment: "The greeting."},
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestRenderer_ContainsServiceName(t *testing.T) {
	root := greetServiceIR()
	html, err := renderer.Render(root)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !strings.Contains(html, "GreetService") {
		t.Error("HTML does not contain service name")
	}
}

func TestRenderer_ContainsServiceComment(t *testing.T) {
	root := greetServiceIR()
	html, err := renderer.Render(root)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !strings.Contains(html, "GreetService provides greeting functionality.") {
		t.Error("HTML does not contain service comment")
	}
}

func TestRenderer_ContainsConnectPath(t *testing.T) {
	root := greetServiceIR()
	html, err := renderer.Render(root)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !strings.Contains(html, "/connectrpc.greet.v1.GreetService/Greet") {
		t.Error("HTML does not contain connect path")
	}
}

func TestRenderer_ContainsHTTPMethod(t *testing.T) {
	root := greetServiceIR()
	html, err := renderer.Render(root)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !strings.Contains(html, "POST") {
		t.Error("HTML does not contain HTTP method")
	}
}

func TestRenderer_ContainsEmbeddedSchema(t *testing.T) {
	root := greetServiceIR()
	html, err := renderer.Render(root)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !strings.Contains(html, "window.__CONNECTVIEW_SCHEMA__") {
		t.Error("HTML does not contain embedded schema JSON")
	}
}

func TestRenderer_ContainsFieldNames(t *testing.T) {
	root := greetServiceIR()
	html, err := renderer.Render(root)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	for _, name := range []string{"name", "locale", "greeting"} {
		if !strings.Contains(html, name) {
			t.Errorf("HTML does not contain field name: %s", name)
		}
	}
}

func TestRenderer_NoExternalDependencies(t *testing.T) {
	root := greetServiceIR()
	html, err := renderer.Render(root)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	for _, ext := range []string{"cdn.", "googleapis.com", "unpkg.com", "jsdelivr.net"} {
		if strings.Contains(html, ext) {
			t.Errorf("HTML contains external dependency: %s", ext)
		}
	}
}

func TestRenderer_IsValidHTML(t *testing.T) {
	root := greetServiceIR()
	html, err := renderer.Render(root)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("HTML missing DOCTYPE")
	}
	if !strings.Contains(html, "<html") {
		t.Error("HTML missing <html> tag")
	}
	if !strings.Contains(html, "</html>") {
		t.Error("HTML missing closing </html> tag")
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/renderer/ -v
```
Expected: FAIL

**Step 3: Create CSS asset**

Create `internal/renderer/assets/style.css` — the full CSS for the viewer matching the design doc's visual spec (sidebar, main content, field colors, etc.).

**Step 4: Create JS asset**

Create `internal/renderer/assets/app.js` — the JavaScript that:
- Reads `window.__CONNECTVIEW_SCHEMA__`
- Builds sidebar navigation
- Generates forms for each RPC
- Handles request sending (ConnectRPC POST/GET)
- Displays responses (JSON pretty-print, error handling)
- Generates curl snippets
- Manages header settings (key/value pairs)
- Handles base URL configuration

**Step 5: Create HTML template**

Create `internal/renderer/assets/index.html.tmpl`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>connectview</title>
  <style>{{.CSS}}</style>
</head>
<body>
  <!-- Header, sidebar, main content rendered by JS from schema -->
  <div id="app"></div>
  <script>window.__CONNECTVIEW_SCHEMA__ = {{.SchemaJSON}};</script>
  <script>{{.JS}}</script>
</body>
</html>
```

**Step 6: Create embed.go**

```go
// internal/renderer/embed.go
package renderer

import "embed"

//go:embed assets/index.html.tmpl
var htmlTemplate string

//go:embed assets/style.css
var cssContent string

//go:embed assets/app.js
var jsContent string
```

**Step 7: Create renderer.go**

```go
// internal/renderer/renderer.go
package renderer

import (
	"bytes"
	"encoding/json"
	"html/template"

	"github.com/Dorayaki-World/connectview/internal/ir"
)

type templateData struct {
	CSS        template.CSS
	JS         template.JS
	SchemaJSON template.JS
}

// Render converts a resolved IR root into a self-contained HTML string.
func Render(root *ir.Root) (string, error) {
	schemaBytes, err := json.Marshal(buildSchema(root))
	if err != nil {
		return "", err
	}

	tmpl, err := template.New("index").Parse(htmlTemplate)
	if err != nil {
		return "", err
	}

	data := templateData{
		CSS:        template.CSS(cssContent),
		JS:         template.JS(jsContent),
		SchemaJSON: template.JS(schemaBytes),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// buildSchema creates the JSON-serializable schema for the frontend.
// This is a simplified view of the IR suitable for the JS app.
func buildSchema(root *ir.Root) any {
	// Convert IR to a JSON-friendly structure
	// ... (implementation builds services with resolved request/response fields)
}
```

The `buildSchema` function converts the IR into a nested JSON structure that the JS app uses to render the UI and build forms. It recursively expands messages (respecting IsRecursive flags) so the JS doesn't need to do resolution.

**Step 8: Run tests**

```bash
go test ./internal/renderer/ -v
```
Expected: PASS

**Step 9: Commit**

```bash
git add internal/renderer/
git commit -m "feat: implement renderer with embedded HTML/CSS/JS assets"
```

---

### Task 6: Implement Entry Point (protoc plugin)

**Files:**
- Modify: `cmd/connectview/main.go`

**Step 1: Implement main.go**

```go
// cmd/connectview/main.go
package main

import (
	"flag"

	"github.com/Dorayaki-World/connectview/internal/parser"
	"github.com/Dorayaki-World/connectview/internal/renderer"
	"github.com/Dorayaki-World/connectview/internal/resolver"
	"google.golang.org/protobuf/compiler/protogen"
)

func main() {
	var flags flag.FlagSet
	protogen.Options{
		ParamFunc: flags.Set,
	}.Run(func(plugin *protogen.Plugin) error {
		// Parse protogen files into IR
		root := parser.Parse(plugin)

		// Resolve cross-file references and detect recursion
		r := resolver.New(root)
		if err := r.Resolve(); err != nil {
			return err
		}

		// Render to HTML
		html, err := renderer.Render(root)
		if err != nil {
			return err
		}

		// Write output file
		outFile := plugin.NewGeneratedFile("index.html", "")
		outFile.P(html)

		return nil
	})
}
```

**Step 2: Verify build**

```bash
go build ./cmd/connectview
```
Expected: SUCCESS

**Step 3: Commit**

```bash
git add cmd/connectview/
git commit -m "feat: implement protoc plugin entry point"
```

---

### Task 7: Create Testdata Proto Files

Create the test proto files exactly as specified in the design doc (Section 12.2).

**Files:**
- Create: `testdata/proto/greet/v1/greet.proto`
- Create: `testdata/proto/user/v1/user.proto`
- Create: `testdata/proto/tree/v1/tree.proto`
- Create: `testdata/proto/mutual/v1/mutual.proto`

Copy the proto files verbatim from the design doc sections 12.2.

**Step 1: Create testdata proto files**

(Content from design doc sections — greet.proto, user.proto, tree.proto, mutual.proto)

**Step 2: Commit**

```bash
git add testdata/
git commit -m "test: add testdata proto files for greet, user, tree, mutual"
```

---

### Task 8: E2E Integration Test

Test the full flow: proto files → protoc with our plugin → HTML output.

**Files:**
- Create: `e2e/e2e_test.go`

**Step 1: Write E2E test**

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

func TestE2E_GenerateHTML(t *testing.T) {
	if _, err := exec.LookPath("protoc"); err != nil {
		t.Skip("protoc not found in PATH, skipping E2E test")
	}

	binaryPath := buildBinary(t)
	tmpDir := t.TempDir()

	cmd := exec.Command("protoc",
		"--plugin=protoc-gen-connectview="+binaryPath,
		"--connectview_out="+tmpDir,
		"-I", "testdata/proto",
		"testdata/proto/greet/v1/greet.proto",
	)
	cmd.Dir = filepath.Join("..")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("protoc failed: %v\nOutput: %s", err, out)
	}

	outFile := filepath.Join(tmpDir, "index.html")
	content, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	html := string(content)

	checks := []string{
		"GreetService",
		"Greet",
		"/connectrpc.greet.v1.GreetService/Greet",
		"GreetRequest",
		"GreetResponse",
		"window.__CONNECTVIEW_SCHEMA__",
	}

	for _, c := range checks {
		if !strings.Contains(html, c) {
			t.Errorf("HTML does not contain: %q", c)
		}
	}

	// Verify no external CDN references
	if strings.Contains(html, "cdn.") {
		t.Error("HTML contains external CDN reference")
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "protoc-gen-connectview")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/connectview")
	cmd.Dir = filepath.Join("..")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\nOutput: %s", err, out)
	}
	return binaryPath
}
```

**Step 2: Run E2E test**

```bash
go test ./e2e/ -v
```
Expected: PASS (if protoc is available)

**Step 3: Commit**

```bash
git add e2e/
git commit -m "test: add E2E integration test for protoc plugin flow"
```

---

### Task 9: Update Design Doc

Update `docs/design.md` to reflect the protogen-based architecture changes.

**Key changes:**
- Section 6.2: Remove `internal/plugin/` and `internal/parser/comment.go`
- Section 6.3: Replace raw stdin/stdout with `protogen.Options{}.Run()`
- Section 7.1: Add `IsOptional`, `IsMap`, `MapKeyType`, `MapValueType`, `MapValueTypeName`, `IsMapEntry` fields
- Section 7.2: Update parser to use protogen types instead of raw descriptorpb
- Note the rename from `window.__PROTOVIEW_SCHEMA__` to `window.__CONNECTVIEW_SCHEMA__`

**Step 1: Update design doc**

**Step 2: Commit**

```bash
git add docs/design.md
git commit -m "docs: update design doc to reflect protogen-based architecture"
```
