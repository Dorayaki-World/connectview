package parser

import (
	"testing"

	"github.com/Dorayaki-World/connectview/internal/ir"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

// newPlugin creates a *protogen.Plugin from FileDescriptorProtos.
// Each file is marked for generation (added to FileToGenerate).
// A default go_package option is added if not already present.
func newPlugin(t *testing.T, fds ...*descriptorpb.FileDescriptorProto) *protogen.Plugin {
	t.Helper()

	req := &pluginpb.CodeGeneratorRequest{}
	for _, fd := range fds {
		// Ensure go_package is set so protogen.Options{}.New() does not fail.
		if fd.GetOptions().GetGoPackage() == "" {
			if fd.Options == nil {
				fd.Options = &descriptorpb.FileOptions{}
			}
			goPkg := fd.GetPackage() + "pb"
			fd.Options.GoPackage = &goPkg
		}
		req.ProtoFile = append(req.ProtoFile, fd)
		req.FileToGenerate = append(req.FileToGenerate, fd.GetName())
	}

	gen, err := protogen.Options{}.New(req)
	if err != nil {
		t.Fatalf("protogen.Options{}.New() failed: %v", err)
	}
	return gen
}

// TestParseService_Basic verifies that a service is parsed correctly,
// including its name, fullName, connectBasePath, and RPC details
// (name, connectPath, httpMethod, request/response typeNames).
func TestParseService_Basic(t *testing.T) {
	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("greet/v1/greet.proto"),
		Package: proto.String("greet.v1"),
		Syntax:  proto.String("proto3"),
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: proto.String("GreetService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       proto.String("Greet"),
						InputType:  proto.String(".greet.v1.GreetRequest"),
						OutputType: proto.String(".greet.v1.GreetResponse"),
					},
				},
			},
		},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("GreetRequest")},
			{Name: proto.String("GreetResponse")},
		},
	}

	plugin := newPlugin(t, fd)
	root := Parse(plugin)

	if len(root.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(root.Services))
	}

	svc := root.Services[0]
	if svc.Name != "GreetService" {
		t.Errorf("service name: got %q, want %q", svc.Name, "GreetService")
	}
	if svc.FullName != "greet.v1.GreetService" {
		t.Errorf("service fullName: got %q, want %q", svc.FullName, "greet.v1.GreetService")
	}
	if svc.ConnectBasePath != "/greet.v1.GreetService/" {
		t.Errorf("connectBasePath: got %q, want %q", svc.ConnectBasePath, "/greet.v1.GreetService/")
	}
	if svc.File != "greet/v1/greet.proto" {
		t.Errorf("file: got %q, want %q", svc.File, "greet/v1/greet.proto")
	}

	if len(svc.RPCs) != 1 {
		t.Fatalf("expected 1 RPC, got %d", len(svc.RPCs))
	}

	rpc := svc.RPCs[0]
	if rpc.Name != "Greet" {
		t.Errorf("rpc name: got %q, want %q", rpc.Name, "Greet")
	}
	if rpc.ConnectPath != "/greet.v1.GreetService/Greet" {
		t.Errorf("connectPath: got %q, want %q", rpc.ConnectPath, "/greet.v1.GreetService/Greet")
	}
	if rpc.HTTPMethod != "POST" {
		t.Errorf("httpMethod: got %q, want %q", rpc.HTTPMethod, "POST")
	}
	if rpc.Request == nil {
		t.Fatal("rpc.Request is nil")
	}
	if rpc.Request.TypeName != ".greet.v1.GreetRequest" {
		t.Errorf("request typeName: got %q, want %q", rpc.Request.TypeName, ".greet.v1.GreetRequest")
	}
	if rpc.Response == nil {
		t.Fatal("rpc.Response is nil")
	}
	if rpc.Response.TypeName != ".greet.v1.GreetResponse" {
		t.Errorf("response typeName: got %q, want %q", rpc.Response.TypeName, ".greet.v1.GreetResponse")
	}
}

// TestParseRPC_IdempotencyLevel_GET verifies that an RPC with
// NO_SIDE_EFFECTS idempotency level is assigned HTTPMethod="GET".
func TestParseRPC_IdempotencyLevel_GET(t *testing.T) {
	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("test.proto"),
		Package: proto.String("test.v1"),
		Syntax:  proto.String("proto3"),
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: proto.String("TestService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       proto.String("GetItem"),
						InputType:  proto.String(".test.v1.GetItemRequest"),
						OutputType: proto.String(".test.v1.GetItemResponse"),
						Options: &descriptorpb.MethodOptions{
							IdempotencyLevel: descriptorpb.MethodOptions_NO_SIDE_EFFECTS.Enum(),
						},
					},
				},
			},
		},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("GetItemRequest")},
			{Name: proto.String("GetItemResponse")},
		},
	}

	plugin := newPlugin(t, fd)
	root := Parse(plugin)

	if len(root.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(root.Services))
	}
	if len(root.Services[0].RPCs) != 1 {
		t.Fatalf("expected 1 RPC, got %d", len(root.Services[0].RPCs))
	}

	rpc := root.Services[0].RPCs[0]
	if rpc.HTTPMethod != "GET" {
		t.Errorf("httpMethod: got %q, want %q", rpc.HTTPMethod, "GET")
	}
}

// TestParseMessage_FieldTypes verifies that fields of different types
// are parsed correctly: string, enum (with typeName), and repeated.
func TestParseMessage_FieldTypes(t *testing.T) {
	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("test.proto"),
		Package: proto.String("test.v1"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("User"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("name"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:     proto.String("role"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
						TypeName: proto.String(".test.v1.Role"),
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
				Name: proto.String("Role"),
				Value: []*descriptorpb.EnumValueDescriptorProto{
					{Name: proto.String("ROLE_UNSPECIFIED"), Number: proto.Int32(0)},
					{Name: proto.String("ROLE_ADMIN"), Number: proto.Int32(1)},
				},
			},
		},
	}

	plugin := newPlugin(t, fd)
	root := Parse(plugin)

	msg, ok := root.Messages[".test.v1.User"]
	if !ok {
		t.Fatalf("message .test.v1.User not found in root.Messages")
	}
	if len(msg.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(msg.Fields))
	}

	// string name = 1;
	f0 := msg.Fields[0]
	if f0.Name != "name" {
		t.Errorf("field 0 name: got %q, want %q", f0.Name, "name")
	}
	if f0.Type != ir.FieldTypeString {
		t.Errorf("field 0 type: got %d, want %d (STRING)", f0.Type, ir.FieldTypeString)
	}
	if f0.Label != ir.FieldLabelOptional {
		t.Errorf("field 0 label: got %d, want %d (OPTIONAL)", f0.Label, ir.FieldLabelOptional)
	}

	// Role role = 2;
	f1 := msg.Fields[1]
	if f1.Name != "role" {
		t.Errorf("field 1 name: got %q, want %q", f1.Name, "role")
	}
	if f1.Type != ir.FieldTypeEnum {
		t.Errorf("field 1 type: got %d, want %d (ENUM)", f1.Type, ir.FieldTypeEnum)
	}
	if f1.TypeName != ".test.v1.Role" {
		t.Errorf("field 1 typeName: got %q, want %q", f1.TypeName, ".test.v1.Role")
	}

	// repeated string tags = 3;
	f2 := msg.Fields[2]
	if f2.Name != "tags" {
		t.Errorf("field 2 name: got %q, want %q", f2.Name, "tags")
	}
	if f2.Label != ir.FieldLabelRepeated {
		t.Errorf("field 2 label: got %d, want %d (REPEATED)", f2.Label, ir.FieldLabelRepeated)
	}
}

// TestParseMessage_Streaming verifies that server/client streaming flags
// are parsed correctly.
func TestParseMessage_Streaming(t *testing.T) {
	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("test.proto"),
		Package: proto.String("test.v1"),
		Syntax:  proto.String("proto3"),
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: proto.String("StreamService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:            proto.String("ServerStream"),
						InputType:       proto.String(".test.v1.StreamReq"),
						OutputType:      proto.String(".test.v1.StreamRes"),
						ServerStreaming:  proto.Bool(true),
						ClientStreaming:  proto.Bool(false),
					},
				},
			},
		},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("StreamReq")},
			{Name: proto.String("StreamRes")},
		},
	}

	plugin := newPlugin(t, fd)
	root := Parse(plugin)

	if len(root.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(root.Services))
	}
	if len(root.Services[0].RPCs) != 1 {
		t.Fatalf("expected 1 RPC, got %d", len(root.Services[0].RPCs))
	}

	rpc := root.Services[0].RPCs[0]
	if !rpc.ServerStreaming {
		t.Error("expected ServerStreaming=true, got false")
	}
	if rpc.ClientStreaming {
		t.Error("expected ClientStreaming=false, got true")
	}
}

// TestParseMessage_Comments verifies that comments are extracted from
// SourceCodeInfo for services, RPCs, messages, and fields.
func TestParseMessage_Comments(t *testing.T) {
	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("test.proto"),
		Package: proto.String("test.v1"),
		Syntax:  proto.String("proto3"),
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: proto.String("CommentService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       proto.String("DoStuff"),
						InputType:  proto.String(".test.v1.Req"),
						OutputType: proto.String(".test.v1.Res"),
					},
				},
			},
		},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("Req"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("query"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
			},
			{Name: proto.String("Res")},
		},
		SourceCodeInfo: &descriptorpb.SourceCodeInfo{
			Location: []*descriptorpb.SourceCodeInfo_Location{
				{
					Path:            []int32{6, 0}, // service index 0
					Span:            []int32{3, 0, 10, 1},
					LeadingComments: proto.String(" Service comment.\n"),
				},
				{
					Path:            []int32{6, 0, 2, 0}, // service 0, method 0
					Span:            []int32{5, 2, 7, 3},
					LeadingComments: proto.String(" RPC comment.\n"),
				},
				{
					Path:            []int32{4, 0}, // message index 0
					Span:            []int32{12, 0, 16, 1},
					LeadingComments: proto.String(" Message comment.\n"),
				},
				{
					Path:            []int32{4, 0, 2, 0}, // message 0, field 0
					Span:            []int32{14, 2, 14, 20},
					LeadingComments: proto.String(" Field comment.\n"),
				},
			},
		},
	}

	plugin := newPlugin(t, fd)
	root := Parse(plugin)

	// Verify service comment
	if len(root.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(root.Services))
	}
	svc := root.Services[0]
	if svc.Comment != "Service comment." {
		t.Errorf("service comment: got %q, want %q", svc.Comment, "Service comment.")
	}

	// Verify RPC comment
	if len(svc.RPCs) != 1 {
		t.Fatalf("expected 1 RPC, got %d", len(svc.RPCs))
	}
	if svc.RPCs[0].Comment != "RPC comment." {
		t.Errorf("rpc comment: got %q, want %q", svc.RPCs[0].Comment, "RPC comment.")
	}

	// Verify message comment
	msg, ok := root.Messages[".test.v1.Req"]
	if !ok {
		t.Fatalf("message .test.v1.Req not found in root.Messages")
	}
	if msg.Comment != "Message comment." {
		t.Errorf("message comment: got %q, want %q", msg.Comment, "Message comment.")
	}

	// Verify field comment
	if len(msg.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(msg.Fields))
	}
	if msg.Fields[0].Comment != "Field comment." {
		t.Errorf("field comment: got %q, want %q", msg.Fields[0].Comment, "Field comment.")
	}
}

// TestParseMessage_MapField verifies map field detection using protogen's
// built-in field.Desc.IsMap().
func TestParseMessage_MapField(t *testing.T) {
	nestedMapEntry := &descriptorpb.DescriptorProto{
		Name: proto.String("MetadataEntry"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{
				Name:   proto.String("key"),
				Number: proto.Int32(1),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			},
			{
				Name:   proto.String("value"),
				Number: proto.Int32(2),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			},
		},
		Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
	}

	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("test.proto"),
		Package: proto.String("test.v1"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("MyMessage"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     proto.String("metadata"),
						Number:   proto.Int32(7),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: proto.String(".test.v1.MyMessage.MetadataEntry"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{nestedMapEntry},
			},
		},
	}

	plugin := newPlugin(t, fd)
	root := Parse(plugin)

	msg, ok := root.Messages[".test.v1.MyMessage"]
	if !ok {
		t.Fatalf("message .test.v1.MyMessage not found in root.Messages")
	}
	if len(msg.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(msg.Fields))
	}

	f := msg.Fields[0]
	if f.Name != "metadata" {
		t.Errorf("field name: got %q, want %q", f.Name, "metadata")
	}
	if !f.IsMap {
		t.Error("expected IsMap=true, got false")
	}
	if f.MapKeyType != ir.FieldTypeString {
		t.Errorf("MapKeyType: got %d, want %d (STRING)", f.MapKeyType, ir.FieldTypeString)
	}
	if f.MapValueType != ir.FieldTypeString {
		t.Errorf("MapValueType: got %d, want %d (STRING)", f.MapValueType, ir.FieldTypeString)
	}
}

// TestParseMessage_OptionalField verifies proto3 optional detection.
func TestParseMessage_OptionalField(t *testing.T) {
	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("test.proto"),
		Package: proto.String("test.v1"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("Msg"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:           proto.String("email"),
						Number:         proto.Int32(1),
						Type:           descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:          descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						OneofIndex:     proto.Int32(0),
						Proto3Optional: proto.Bool(true),
					},
				},
				OneofDecl: []*descriptorpb.OneofDescriptorProto{
					{Name: proto.String("_email")}, // synthetic oneof
				},
			},
		},
	}

	plugin := newPlugin(t, fd)
	root := Parse(plugin)

	msg, ok := root.Messages[".test.v1.Msg"]
	if !ok {
		t.Fatalf("message .test.v1.Msg not found in root.Messages")
	}
	if len(msg.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(msg.Fields))
	}

	f := msg.Fields[0]
	if f.Name != "email" {
		t.Errorf("field name: got %q, want %q", f.Name, "email")
	}
	if !f.IsOptional {
		t.Error("expected IsOptional=true, got false")
	}
	if f.OneofName != "" {
		t.Errorf("expected OneofName to be empty for proto3 optional, got %q", f.OneofName)
	}
}

// TestParseMessage_Oneof verifies oneof field grouping.
func TestParseMessage_Oneof(t *testing.T) {
	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("test.proto"),
		Package: proto.String("test.v1"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("Event"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("id"),
						Number: proto.Int32(1),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
					{
						Name:       proto.String("text_payload"),
						Number:     proto.Int32(2),
						Type:       descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:      descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						OneofIndex: proto.Int32(0),
					},
					{
						Name:       proto.String("binary_payload"),
						Number:     proto.Int32(3),
						Type:       descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum(),
						Label:      descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						OneofIndex: proto.Int32(0),
					},
				},
				OneofDecl: []*descriptorpb.OneofDescriptorProto{
					{Name: proto.String("payload")},
				},
			},
		},
	}

	plugin := newPlugin(t, fd)
	root := Parse(plugin)

	msg, ok := root.Messages[".test.v1.Event"]
	if !ok {
		t.Fatalf("message .test.v1.Event not found in root.Messages")
	}
	if len(msg.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(msg.Fields))
	}

	// id - regular field, not in a oneof
	f0 := msg.Fields[0]
	if f0.Name != "id" {
		t.Errorf("field 0 name: got %q, want %q", f0.Name, "id")
	}
	if f0.OneofName != "" {
		t.Errorf("field 0 OneofName: got %q, want empty", f0.OneofName)
	}

	// text_payload - in oneof "payload"
	f1 := msg.Fields[1]
	if f1.Name != "text_payload" {
		t.Errorf("field 1 name: got %q, want %q", f1.Name, "text_payload")
	}
	if f1.OneofName != "payload" {
		t.Errorf("field 1 OneofName: got %q, want %q", f1.OneofName, "payload")
	}

	// binary_payload - in oneof "payload"
	f2 := msg.Fields[2]
	if f2.Name != "binary_payload" {
		t.Errorf("field 2 name: got %q, want %q", f2.Name, "binary_payload")
	}
	if f2.OneofName != "payload" {
		t.Errorf("field 2 OneofName: got %q, want %q", f2.OneofName, "payload")
	}
}

// TestParseFile verifies that files are parsed correctly.
func TestParseFile(t *testing.T) {
	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("greet/v1/greet.proto"),
		Package: proto.String("greet.v1"),
		Syntax:  proto.String("proto3"),
	}

	plugin := newPlugin(t, fd)
	root := Parse(plugin)

	if len(root.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(root.Files))
	}

	f := root.Files[0]
	if f.Name != "greet/v1/greet.proto" {
		t.Errorf("file name: got %q, want %q", f.Name, "greet/v1/greet.proto")
	}
	if f.Package != "greet.v1" {
		t.Errorf("file package: got %q, want %q", f.Package, "greet.v1")
	}
}

// TestParseSkipsNonGenerateFiles verifies that files not marked for
// generation are skipped.
func TestParseSkipsNonGenerateFiles(t *testing.T) {
	fd1 := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("dep.proto"),
		Package: proto.String("dep.v1"),
		Syntax:  proto.String("proto3"),
		Options: &descriptorpb.FileOptions{GoPackage: proto.String("dep.v1pb")},
	}
	fd2 := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("main.proto"),
		Package: proto.String("main.v1"),
		Syntax:  proto.String("proto3"),
		Options: &descriptorpb.FileOptions{GoPackage: proto.String("main.v1pb")},
	}

	req := &pluginpb.CodeGeneratorRequest{
		ProtoFile:      []*descriptorpb.FileDescriptorProto{fd1, fd2},
		FileToGenerate: []string{"main.proto"}, // only main.proto should be processed
	}

	gen, err := protogen.Options{}.New(req)
	if err != nil {
		t.Fatalf("protogen.Options{}.New() failed: %v", err)
	}

	root := Parse(gen)

	if len(root.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(root.Files))
	}
	if root.Files[0].Name != "main.proto" {
		t.Errorf("expected main.proto, got %q", root.Files[0].Name)
	}
}

// TestParseEnum verifies that an enum is parsed correctly.
func TestParseEnum(t *testing.T) {
	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("test.proto"),
		Package: proto.String("test.v1"),
		Syntax:  proto.String("proto3"),
		EnumType: []*descriptorpb.EnumDescriptorProto{
			{
				Name: proto.String("Status"),
				Value: []*descriptorpb.EnumValueDescriptorProto{
					{Name: proto.String("STATUS_UNSPECIFIED"), Number: proto.Int32(0)},
					{Name: proto.String("STATUS_ACTIVE"), Number: proto.Int32(1)},
					{Name: proto.String("STATUS_INACTIVE"), Number: proto.Int32(2)},
				},
			},
		},
	}

	plugin := newPlugin(t, fd)
	root := Parse(plugin)

	enum, ok := root.Enums[".test.v1.Status"]
	if !ok {
		t.Fatalf("enum .test.v1.Status not found in root.Enums")
	}
	if enum.Name != "Status" {
		t.Errorf("enum name: got %q, want %q", enum.Name, "Status")
	}
	if enum.FullName != ".test.v1.Status" {
		t.Errorf("enum fullName: got %q, want %q", enum.FullName, ".test.v1.Status")
	}
	if len(enum.Values) != 3 {
		t.Fatalf("expected 3 enum values, got %d", len(enum.Values))
	}

	expectedValues := []struct {
		name   string
		number int32
	}{
		{"STATUS_UNSPECIFIED", 0},
		{"STATUS_ACTIVE", 1},
		{"STATUS_INACTIVE", 2},
	}
	for i, ev := range expectedValues {
		v := enum.Values[i]
		if v.Name != ev.name {
			t.Errorf("value %d name: got %q, want %q", i, v.Name, ev.name)
		}
		if v.Number != ev.number {
			t.Errorf("value %d number: got %d, want %d", i, v.Number, ev.number)
		}
	}
}

// TestParseMessage_Nested verifies that nested messages are registered
// in root.Messages with correct FQNs.
func TestParseMessage_Nested(t *testing.T) {
	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("test.proto"),
		Package: proto.String("test.v1"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("Outer"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     proto.String("inner"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: proto.String(".test.v1.Outer.Inner"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("Inner"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   proto.String("value"),
								Number: proto.Int32(1),
								Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
								Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
				},
			},
		},
	}

	plugin := newPlugin(t, fd)
	root := Parse(plugin)

	outer, ok := root.Messages[".test.v1.Outer"]
	if !ok {
		t.Fatalf("message .test.v1.Outer not found in root.Messages")
	}
	if outer.FullName != ".test.v1.Outer" {
		t.Errorf("outer fullName: got %q, want %q", outer.FullName, ".test.v1.Outer")
	}

	inner, ok := root.Messages[".test.v1.Outer.Inner"]
	if !ok {
		t.Fatalf("nested message .test.v1.Outer.Inner not found in root.Messages")
	}
	if inner.FullName != ".test.v1.Outer.Inner" {
		t.Errorf("inner fullName: got %q, want %q", inner.FullName, ".test.v1.Outer.Inner")
	}
	if inner.Name != "Inner" {
		t.Errorf("inner name: got %q, want %q", inner.Name, "Inner")
	}
	if len(inner.Fields) != 1 {
		t.Fatalf("inner fields: expected 1, got %d", len(inner.Fields))
	}
	if inner.Fields[0].Name != "value" {
		t.Errorf("inner field name: got %q, want %q", inner.Fields[0].Name, "value")
	}
}
