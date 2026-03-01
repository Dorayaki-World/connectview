package parser

import (
	"strings"

	"github.com/Dorayaki-World/connectview/internal/ir"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Parse converts a *protogen.Plugin into the intermediate representation.
// Only files where f.Generate == true are processed.
func Parse(plugin *protogen.Plugin) *ir.Root {
	root := &ir.Root{
		Messages: make(map[string]*ir.Message),
		Enums:    make(map[string]*ir.Enum),
	}

	for _, f := range plugin.Files {
		// Register messages and enums from ALL files (including imports)
		// so that cross-file references (e.g. google.protobuf.Timestamp)
		// can be resolved.
		for _, enum := range f.Enums {
			e := parseEnum(enum)
			root.Enums[e.FullName] = e
		}
		for _, msg := range f.Messages {
			parseMessageRecursive(msg, root)
		}

		if !f.Generate {
			continue
		}

		file := &ir.File{
			Name:    f.Desc.Path(),
			Package: string(f.Desc.Package()),
		}
		root.Files = append(root.Files, file)

		// Parse services (only from files marked for generation).
		for _, svc := range f.Services {
			s := parseService(svc, f)
			root.Services = append(root.Services, s)
		}
	}

	return root
}

func parseService(svc *protogen.Service, f *protogen.File) *ir.Service {
	fullName := string(svc.Desc.FullName())

	s := &ir.Service{
		Name:            string(svc.Desc.Name()),
		FullName:        fullName,
		File:            f.Desc.Path(),
		Comment:         trimComment(string(svc.Comments.Leading)),
		ConnectBasePath: "/" + fullName + "/",
	}

	for _, method := range svc.Methods {
		rpc := parseRPC(method, fullName)
		s.RPCs = append(s.RPCs, rpc)
	}

	return s
}

func parseRPC(method *protogen.Method, serviceFQN string) *ir.RPC {
	httpMethod := "POST"
	methodOpts, ok := method.Desc.Options().(*descriptorpb.MethodOptions)
	if ok && methodOpts != nil &&
		methodOpts.GetIdempotencyLevel() == descriptorpb.MethodOptions_NO_SIDE_EFFECTS {
		httpMethod = "GET"
	}

	inputFQN := "." + string(method.Input.Desc.FullName())
	outputFQN := "." + string(method.Output.Desc.FullName())

	return &ir.RPC{
		Name:        string(method.Desc.Name()),
		Comment:     trimComment(string(method.Comments.Leading)),
		ConnectPath: "/" + serviceFQN + "/" + string(method.Desc.Name()),
		HTTPMethod:  httpMethod,
		Request: &ir.MessageRef{
			TypeName: inputFQN,
		},
		Response: &ir.MessageRef{
			TypeName: outputFQN,
		},
		ClientStreaming: method.Desc.IsStreamingClient(),
		ServerStreaming: method.Desc.IsStreamingServer(),
	}
}

// parseMessageRecursive parses a protogen.Message and all its nested
// messages and enums, registering them in root.
func parseMessageRecursive(msg *protogen.Message, root *ir.Root) *ir.Message {
	// Skip map entry messages.
	if msg.Desc.IsMapEntry() {
		return nil
	}

	fullName := "." + string(msg.Desc.FullName())

	m := &ir.Message{
		Name:     string(msg.Desc.Name()),
		FullName: fullName,
		Comment:  trimComment(string(msg.Comments.Leading)),
	}

	// Parse nested enums.
	for _, enum := range msg.Enums {
		e := parseEnum(enum)
		m.NestedEnums = append(m.NestedEnums, e)
		root.Enums[e.FullName] = e
	}

	// Parse nested messages (recursively).
	for _, nested := range msg.Messages {
		nestedMsg := parseMessageRecursive(nested, root)
		if nestedMsg != nil { // nil means it was a map entry
			m.NestedMessages = append(m.NestedMessages, nestedMsg)
		}
	}

	// Parse fields.
	for _, field := range msg.Fields {
		f := parseField(field)
		m.Fields = append(m.Fields, f)
	}

	root.Messages[fullName] = m
	return m
}

func parseField(field *protogen.Field) *ir.Field {
	f := &ir.Field{
		Name:    string(field.Desc.Name()),
		Number:  int32(field.Desc.Number()),
		Type:    protoKindToFieldType(field.Desc.Kind()),
		Label:   protoCardinalityToLabel(field.Desc.Cardinality()),
		Comment: trimComment(string(field.Comments.Leading)),
	}

	// Set TypeName for message and enum types.
	if field.Desc.Kind() == protoreflect.MessageKind || field.Desc.Kind() == protoreflect.GroupKind {
		f.TypeName = "." + string(field.Desc.Message().FullName())
	} else if field.Desc.Kind() == protoreflect.EnumKind {
		f.TypeName = "." + string(field.Desc.Enum().FullName())
	}

	// Check for map fields using protogen's built-in detection.
	if field.Desc.IsMap() {
		f.IsMap = true
		f.Type = ir.FieldTypeMessage // map fields are message type
		f.Label = ir.FieldLabelRepeated

		// Extract key/value types from the map entry message fields.
		keyField := field.Message.Fields[0]
		valueField := field.Message.Fields[1]

		f.MapKeyType = protoKindToFieldType(keyField.Desc.Kind())
		f.MapValueType = protoKindToFieldType(valueField.Desc.Kind())
		if valueField.Desc.Kind() == protoreflect.MessageKind {
			f.MapValueTypeName = "." + string(valueField.Desc.Message().FullName())
		} else if valueField.Desc.Kind() == protoreflect.EnumKind {
			f.MapValueTypeName = "." + string(valueField.Desc.Enum().FullName())
		}

		// TypeName for map fields points to the synthetic MapEntry message.
		f.TypeName = "." + string(field.Desc.Message().FullName())
	}

	// Check for proto3 optional.
	if field.Desc.HasOptionalKeyword() {
		f.IsOptional = true
	}

	// Set OneofName for real (non-synthetic) oneofs.
	if field.Oneof != nil && !field.Oneof.Desc.IsSynthetic() {
		f.OneofName = string(field.Oneof.Desc.Name())
	}

	return f
}

func parseEnum(enum *protogen.Enum) *ir.Enum {
	fullName := "." + string(enum.Desc.FullName())

	e := &ir.Enum{
		Name:     string(enum.Desc.Name()),
		FullName: fullName,
		Comment:  trimComment(string(enum.Comments.Leading)),
	}

	for _, v := range enum.Values {
		ev := &ir.EnumValue{
			Name:    string(v.Desc.Name()),
			Number:  int32(v.Desc.Number()),
			Comment: trimComment(string(v.Comments.Leading)),
		}
		e.Values = append(e.Values, ev)
	}

	return e
}

// protoKindToFieldType maps protoreflect.Kind to ir.FieldType.
func protoKindToFieldType(kind protoreflect.Kind) ir.FieldType {
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
		return 0
	}
}

// protoCardinalityToLabel maps protoreflect.Cardinality to ir.FieldLabel.
func protoCardinalityToLabel(c protoreflect.Cardinality) ir.FieldLabel {
	switch c {
	case protoreflect.Optional:
		return ir.FieldLabelOptional
	case protoreflect.Required:
		return ir.FieldLabelRequired
	case protoreflect.Repeated:
		return ir.FieldLabelRepeated
	default:
		return ir.FieldLabelOptional
	}
}

// trimComment trims whitespace from a comment string.
func trimComment(s string) string {
	return strings.TrimSpace(s)
}
