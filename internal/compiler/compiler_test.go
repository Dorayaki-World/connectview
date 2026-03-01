package compiler_test

import (
	"testing"

	"github.com/Dorayaki-World/connectview/internal/compiler"
)

func TestCompile_BasicProto(t *testing.T) {
	root, err := compiler.Compile([]string{"../../testdata/proto"}, []string{
		"greet/v1/greet.proto",
	})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if len(root.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(root.Services))
	}
	if root.Services[0].Name != "GreetService" {
		t.Errorf("expected GreetService, got %s", root.Services[0].Name)
	}
	if len(root.Services[0].RPCs) != 2 {
		t.Errorf("expected 2 RPCs, got %d", len(root.Services[0].RPCs))
	}

	if _, ok := root.Messages[".connectrpc.greet.v1.GreetRequest"]; !ok {
		t.Error("GreetRequest message not found")
	}
}

func TestCompile_MultipleFiles(t *testing.T) {
	root, err := compiler.Compile([]string{"../../testdata/proto"}, []string{
		"greet/v1/greet.proto",
		"user/v1/user.proto",
	})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if len(root.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(root.Services))
	}
}

func TestCompile_InvalidPath(t *testing.T) {
	_, err := compiler.Compile([]string{"../../testdata/proto"}, []string{
		"nonexistent/v1/nope.proto",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent proto file")
	}
}
