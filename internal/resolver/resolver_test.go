package resolver_test

import (
	"testing"

	"github.com/Dorayaki-World/connectview/internal/ir"
	"github.com/Dorayaki-World/connectview/internal/resolver"
)

func buildRoot(services []*ir.Service, messages map[string]*ir.Message, enums map[string]*ir.Enum) *ir.Root {
	if messages == nil {
		messages = make(map[string]*ir.Message)
	}
	if enums == nil {
		enums = make(map[string]*ir.Enum)
	}
	return &ir.Root{
		Services: services,
		Messages: messages,
		Enums:    enums,
	}
}

func TestResolve_BasicMessageRef(t *testing.T) {
	reqMsg := &ir.Message{Name: "GreetRequest", FullName: ".greet.v1.GreetRequest"}
	respMsg := &ir.Message{Name: "GreetResponse", FullName: ".greet.v1.GreetResponse"}

	root := buildRoot(
		[]*ir.Service{
			{
				Name:     "GreetService",
				FullName: "greet.v1.GreetService",
				RPCs: []*ir.RPC{
					{
						Name:     "Greet",
						Request:  &ir.MessageRef{TypeName: ".greet.v1.GreetRequest"},
						Response: &ir.MessageRef{TypeName: ".greet.v1.GreetResponse"},
					},
				},
			},
		},
		map[string]*ir.Message{
			".greet.v1.GreetRequest":  reqMsg,
			".greet.v1.GreetResponse": respMsg,
		},
		nil,
	)

	r := resolver.New(root)
	if err := r.Resolve(); err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}

	rpc := root.Services[0].RPCs[0]
	if rpc.Request.Resolved == nil {
		t.Fatal("Request.Resolved is nil")
	}
	if rpc.Request.Resolved != reqMsg {
		t.Errorf("Request.Resolved points to wrong message: got %q, want %q", rpc.Request.Resolved.FullName, reqMsg.FullName)
	}
	if rpc.Response.Resolved == nil {
		t.Fatal("Response.Resolved is nil")
	}
	if rpc.Response.Resolved != respMsg {
		t.Errorf("Response.Resolved points to wrong message: got %q, want %q", rpc.Response.Resolved.FullName, respMsg.FullName)
	}
}

func TestResolve_NestedMessageField(t *testing.T) {
	userMsg := &ir.Message{Name: "User", FullName: ".greet.v1.User"}
	respMsg := &ir.Message{
		Name:     "GreetResponse",
		FullName: ".greet.v1.GreetResponse",
		Fields: []*ir.Field{
			{
				Name:     "user",
				Type:     ir.FieldTypeMessage,
				TypeName: ".greet.v1.User",
			},
		},
	}

	root := buildRoot(
		[]*ir.Service{
			{
				Name: "GreetService",
				RPCs: []*ir.RPC{
					{
						Name:     "Greet",
						Request:  &ir.MessageRef{TypeName: ".greet.v1.GreetResponse"},
						Response: &ir.MessageRef{TypeName: ".greet.v1.GreetResponse"},
					},
				},
			},
		},
		map[string]*ir.Message{
			".greet.v1.GreetResponse": respMsg,
			".greet.v1.User":          userMsg,
		},
		nil,
	)

	r := resolver.New(root)
	if err := r.Resolve(); err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}

	userField := respMsg.Fields[0]
	if userField.ResolvedMessage == nil {
		t.Fatal("user field ResolvedMessage is nil")
	}
	if userField.ResolvedMessage != userMsg {
		t.Errorf("user field ResolvedMessage points to wrong message: got %q, want %q", userField.ResolvedMessage.FullName, userMsg.FullName)
	}
}

func TestResolve_EnumField(t *testing.T) {
	statusEnum := &ir.Enum{
		Name:     "Status",
		FullName: ".greet.v1.Status",
		Values: []*ir.EnumValue{
			{Name: "STATUS_UNSPECIFIED", Number: 0},
			{Name: "STATUS_ACTIVE", Number: 1},
		},
	}
	reqMsg := &ir.Message{
		Name:     "GetUserRequest",
		FullName: ".greet.v1.GetUserRequest",
		Fields: []*ir.Field{
			{
				Name:     "status",
				Type:     ir.FieldTypeEnum,
				TypeName: ".greet.v1.Status",
			},
		},
	}

	root := buildRoot(
		[]*ir.Service{
			{
				Name: "UserService",
				RPCs: []*ir.RPC{
					{
						Name:     "GetUser",
						Request:  &ir.MessageRef{TypeName: ".greet.v1.GetUserRequest"},
						Response: &ir.MessageRef{TypeName: ".greet.v1.GetUserRequest"},
					},
				},
			},
		},
		map[string]*ir.Message{
			".greet.v1.GetUserRequest": reqMsg,
		},
		map[string]*ir.Enum{
			".greet.v1.Status": statusEnum,
		},
	)

	r := resolver.New(root)
	if err := r.Resolve(); err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}

	statusField := reqMsg.Fields[0]
	if statusField.ResolvedEnum == nil {
		t.Fatal("status field ResolvedEnum is nil")
	}
	if statusField.ResolvedEnum != statusEnum {
		t.Errorf("status field ResolvedEnum points to wrong enum: got %q, want %q", statusField.ResolvedEnum.FullName, statusEnum.FullName)
	}
}

func TestResolve_DirectRecursion(t *testing.T) {
	treeNode := &ir.Message{
		Name:     "TreeNode",
		FullName: ".tree.v1.TreeNode",
		Fields: []*ir.Field{
			{
				Name:     "value",
				Type:     ir.FieldTypeString,
				TypeName: "",
			},
			{
				Name:     "children",
				Type:     ir.FieldTypeMessage,
				TypeName: ".tree.v1.TreeNode",
				Label:    ir.FieldLabelRepeated,
			},
		},
	}

	root := buildRoot(
		[]*ir.Service{
			{
				Name: "TreeService",
				RPCs: []*ir.RPC{
					{
						Name:     "GetTree",
						Request:  &ir.MessageRef{TypeName: ".tree.v1.TreeNode"},
						Response: &ir.MessageRef{TypeName: ".tree.v1.TreeNode"},
					},
				},
			},
		},
		map[string]*ir.Message{
			".tree.v1.TreeNode": treeNode,
		},
		nil,
	)

	r := resolver.New(root)
	if err := r.Resolve(); err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}

	valueField := treeNode.Fields[0]
	childrenField := treeNode.Fields[1]

	if childrenField.IsRecursive != true {
		t.Error("children field should be marked as recursive")
	}
	if childrenField.ResolvedMessage == nil {
		t.Error("children field ResolvedMessage should not be nil (type info should still be set)")
	}
	if childrenField.ResolvedMessage != treeNode {
		t.Errorf("children field ResolvedMessage should point to TreeNode, got %q", childrenField.ResolvedMessage.FullName)
	}
	if valueField.IsRecursive != false {
		t.Error("value field (string) should NOT be marked as recursive")
	}
}

func TestResolve_MutualRecursion(t *testing.T) {
	nodeA := &ir.Message{
		Name:     "NodeA",
		FullName: ".graph.v1.NodeA",
		Fields: []*ir.Field{
			{
				Name:     "b",
				Type:     ir.FieldTypeMessage,
				TypeName: ".graph.v1.NodeB",
			},
		},
	}
	nodeB := &ir.Message{
		Name:     "NodeB",
		FullName: ".graph.v1.NodeB",
		Fields: []*ir.Field{
			{
				Name:     "a",
				Type:     ir.FieldTypeMessage,
				TypeName: ".graph.v1.NodeA",
			},
		},
	}

	root := buildRoot(
		[]*ir.Service{
			{
				Name: "GraphService",
				RPCs: []*ir.RPC{
					{
						Name:     "GetGraph",
						Request:  &ir.MessageRef{TypeName: ".graph.v1.NodeA"},
						Response: &ir.MessageRef{TypeName: ".graph.v1.NodeA"},
					},
				},
			},
		},
		map[string]*ir.Message{
			".graph.v1.NodeA": nodeA,
			".graph.v1.NodeB": nodeB,
		},
		nil,
	)

	r := resolver.New(root)
	if err := r.Resolve(); err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}

	fieldB := nodeA.Fields[0]
	fieldA := nodeB.Fields[0]

	// NodeA.b is NOT recursive (first time seeing NodeB on the path)
	if fieldB.IsRecursive != false {
		t.Error("NodeA.b should NOT be marked as recursive (first time seeing NodeB)")
	}
	if fieldB.ResolvedMessage == nil {
		t.Error("NodeA.b ResolvedMessage should not be nil")
	}

	// NodeB.a IS recursive (NodeA already on the path)
	if fieldA.IsRecursive != true {
		t.Error("NodeB.a should be marked as recursive (NodeA already on path)")
	}
	if fieldA.ResolvedMessage == nil {
		t.Error("NodeB.a ResolvedMessage should not be nil (type info should still be set)")
	}
	if fieldA.ResolvedMessage != nodeA {
		t.Errorf("NodeB.a ResolvedMessage should point to NodeA, got %q", fieldA.ResolvedMessage.FullName)
	}
}

func TestResolve_SiblingsSameType_NotRecursive(t *testing.T) {
	userMsg := &ir.Message{
		Name:     "User",
		FullName: ".app.v1.User",
		Fields: []*ir.Field{
			{
				Name: "name",
				Type: ir.FieldTypeString,
			},
		},
	}
	respMsg := &ir.Message{
		Name:     "Response",
		FullName: ".app.v1.Response",
		Fields: []*ir.Field{
			{
				Name:     "created_by",
				Type:     ir.FieldTypeMessage,
				TypeName: ".app.v1.User",
			},
			{
				Name:     "updated_by",
				Type:     ir.FieldTypeMessage,
				TypeName: ".app.v1.User",
			},
		},
	}

	root := buildRoot(
		[]*ir.Service{
			{
				Name: "AppService",
				RPCs: []*ir.RPC{
					{
						Name:     "GetData",
						Request:  &ir.MessageRef{TypeName: ".app.v1.Response"},
						Response: &ir.MessageRef{TypeName: ".app.v1.Response"},
					},
				},
			},
		},
		map[string]*ir.Message{
			".app.v1.Response": respMsg,
			".app.v1.User":    userMsg,
		},
		nil,
	)

	r := resolver.New(root)
	if err := r.Resolve(); err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}

	createdBy := respMsg.Fields[0]
	updatedBy := respMsg.Fields[1]

	if createdBy.IsRecursive {
		t.Error("created_by should NOT be marked as recursive")
	}
	if updatedBy.IsRecursive {
		t.Error("updated_by should NOT be marked as recursive")
	}
	if createdBy.ResolvedMessage == nil {
		t.Error("created_by ResolvedMessage should not be nil")
	}
	if createdBy.ResolvedMessage != userMsg {
		t.Errorf("created_by ResolvedMessage should point to User, got %q", createdBy.ResolvedMessage.FullName)
	}
	if updatedBy.ResolvedMessage == nil {
		t.Error("updated_by ResolvedMessage should not be nil")
	}
	if updatedBy.ResolvedMessage != userMsg {
		t.Errorf("updated_by ResolvedMessage should point to User, got %q", updatedBy.ResolvedMessage.FullName)
	}
}
