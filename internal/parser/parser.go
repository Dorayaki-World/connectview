package parser

import (
	"strings"

	"github.com/Dorayaki-World/connectview/internal/ir"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Parse converts a slice of FileDescriptorProto into the intermediate representation.
func Parse(fds []*descriptorpb.FileDescriptorProto) (*ir.Root, error) {
	root := &ir.Root{
		Messages: make(map[string]*ir.Message),
		Enums:    make(map[string]*ir.Enum),
	}

	for _, fd := range fds {
		file := &ir.File{
			Name:    fd.GetName(),
			Package: fd.GetPackage(),
		}
		root.Files = append(root.Files, file)

		prefix := "." + fd.GetPackage()

		// Parse services (field number 6 in FileDescriptorProto).
		for i, sd := range fd.GetService() {
			svc := parseService(fd, sd, i)
			root.Services = append(root.Services, svc)
		}

		// Parse top-level messages (field number 4 in FileDescriptorProto).
		for i, md := range fd.GetMessageType() {
			msgPath := []int32{4, int32(i)}
			msg := parseMessage(fd, md, prefix, msgPath, root)
			root.Messages[msg.FullName] = msg
		}

		// Parse top-level enums (field number 5 in FileDescriptorProto).
		for i, ed := range fd.GetEnumType() {
			enumPath := []int32{5, int32(i)}
			enum := parseEnum(fd, ed, prefix, enumPath)
			root.Enums[enum.FullName] = enum
		}
	}

	return root, nil
}

func parseService(fd *descriptorpb.FileDescriptorProto, sd *descriptorpb.ServiceDescriptorProto, serviceIndex int) *ir.Service {
	fullName := fd.GetPackage() + "." + sd.GetName()

	svc := &ir.Service{
		Name:            sd.GetName(),
		FullName:        fullName,
		File:            fd.GetName(),
		Comment:         extractComment(fd, []int32{6, int32(serviceIndex)}),
		ConnectBasePath: "/" + fullName + "/",
	}

	for i, md := range sd.GetMethod() {
		rpc := parseRPC(fd, md, fullName, serviceIndex, i)
		svc.RPCs = append(svc.RPCs, rpc)
	}

	return svc
}

func parseRPC(fd *descriptorpb.FileDescriptorProto, md *descriptorpb.MethodDescriptorProto, serviceFQN string, serviceIndex, methodIndex int) *ir.RPC {
	httpMethod := "POST"
	if md.GetOptions() != nil &&
		md.GetOptions().GetIdempotencyLevel() == descriptorpb.MethodOptions_NO_SIDE_EFFECTS {
		httpMethod = "GET"
	}

	return &ir.RPC{
		Name:        md.GetName(),
		Comment:     extractComment(fd, []int32{6, int32(serviceIndex), 2, int32(methodIndex)}),
		ConnectPath: "/" + serviceFQN + "/" + md.GetName(),
		HTTPMethod:  httpMethod,
		Request: &ir.MessageRef{
			TypeName: md.GetInputType(),
		},
		Response: &ir.MessageRef{
			TypeName: md.GetOutputType(),
		},
		ClientStreaming: md.GetClientStreaming(),
		ServerStreaming: md.GetServerStreaming(),
	}
}

func parseMessage(fd *descriptorpb.FileDescriptorProto, md *descriptorpb.DescriptorProto, prefix string, msgPath []int32, root *ir.Root) *ir.Message {
	fullName := prefix + "." + md.GetName()

	msg := &ir.Message{
		Name:     md.GetName(),
		FullName: fullName,
		Comment:  extractComment(fd, msgPath),
	}

	// Parse nested messages (field number 3 in DescriptorProto).
	for i, nested := range md.GetNestedType() {
		// Skip MapEntry synthetic messages.
		if nested.GetOptions() != nil && nested.GetOptions().GetMapEntry() {
			continue
		}
		nestedPath := append(msgPath, 3, int32(i))
		nestedMsg := parseMessage(fd, nested, fullName, nestedPath, root)
		msg.NestedMessages = append(msg.NestedMessages, nestedMsg)
		root.Messages[nestedMsg.FullName] = nestedMsg
	}

	// Parse nested enums (field number 4 in DescriptorProto).
	for i, ed := range md.GetEnumType() {
		enumPath := append(msgPath, 4, int32(i))
		enum := parseEnum(fd, ed, fullName, enumPath)
		msg.NestedEnums = append(msg.NestedEnums, enum)
		root.Enums[enum.FullName] = enum
	}

	// Parse fields (field number 2 in DescriptorProto).
	for i, field := range md.GetField() {
		fieldPath := append(msgPath, 2, int32(i))
		f := parseField(fd, field, md, fieldPath)
		msg.Fields = append(msg.Fields, f)
	}

	return msg
}

func parseField(fd *descriptorpb.FileDescriptorProto, field *descriptorpb.FieldDescriptorProto, parentMsg *descriptorpb.DescriptorProto, fieldPath []int32) *ir.Field {
	f := &ir.Field{
		Name:     field.GetName(),
		Number:   field.GetNumber(),
		Type:     ir.FieldType(field.GetType()),
		TypeName: field.GetTypeName(),
		Label:    ir.FieldLabel(field.GetLabel()),
		Comment:  extractComment(fd, fieldPath),
	}

	// Check for map fields: the field must be of type MESSAGE with label REPEATED,
	// and the referenced type must be a synthetic MapEntry message.
	if field.GetType() == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE &&
		field.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED {
		for _, nested := range parentMsg.GetNestedType() {
			if nested.GetOptions() != nil && nested.GetOptions().GetMapEntry() &&
				strings.HasSuffix(field.GetTypeName(), "."+nested.GetName()) {
				f.IsMap = true
				for _, mapField := range nested.GetField() {
					switch mapField.GetNumber() {
					case 1: // key
						f.MapKeyType = ir.FieldType(mapField.GetType())
					case 2: // value
						f.MapValueType = ir.FieldType(mapField.GetType())
						f.MapValueTypeName = mapField.GetTypeName()
					}
				}
				break
			}
		}
	}

	// Check for proto3 optional.
	if field.GetProto3Optional() {
		f.IsOptional = true
		// Do not set OneofName for proto3 optional fields.
	} else if field.OneofIndex != nil {
		// Regular oneof field.
		idx := field.GetOneofIndex()
		if int(idx) < len(parentMsg.GetOneofDecl()) {
			f.OneofName = parentMsg.GetOneofDecl()[idx].GetName()
		}
	}

	return f
}

func parseEnum(fd *descriptorpb.FileDescriptorProto, ed *descriptorpb.EnumDescriptorProto, prefix string, enumPath []int32) *ir.Enum {
	fullName := prefix + "." + ed.GetName()

	enum := &ir.Enum{
		Name:     ed.GetName(),
		FullName: fullName,
		Comment:  extractComment(fd, enumPath),
	}

	// Parse enum values (field number 2 in EnumDescriptorProto).
	for i, v := range ed.GetValue() {
		valuePath := append(enumPath, 2, int32(i))
		ev := &ir.EnumValue{
			Name:    v.GetName(),
			Number:  v.GetNumber(),
			Comment: extractComment(fd, valuePath),
		}
		enum.Values = append(enum.Values, ev)
	}

	return enum
}
