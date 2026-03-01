package compiler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Dorayaki-World/connectview/internal/ir"
	"github.com/Dorayaki-World/connectview/internal/parser"
	"github.com/Dorayaki-World/connectview/internal/resolver"
	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	pluginpb "google.golang.org/protobuf/types/pluginpb"
)

// Compile parses .proto files from the given import paths and returns a resolved IR.
func Compile(importPaths []string, files []string) (*ir.Root, error) {
	comp := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(
			&protocompile.SourceResolver{
				ImportPaths: importPaths,
			},
		),
		SourceInfoMode: protocompile.SourceInfoStandard,
	}

	ctx := context.Background()
	compiled, err := comp.Compile(ctx, files...)
	if err != nil {
		return nil, fmt.Errorf("proto compilation failed: %w", err)
	}

	// Collect ALL file descriptors (compiled files + transitive deps)
	// by walking each compiled file's import graph.
	seen := make(map[string]bool)
	var fdProtos []*descriptorpb.FileDescriptorProto

	for _, f := range compiled {
		collectFileDescriptors(f, seen, &fdProtos)
	}

	// Build a synthetic CodeGeneratorRequest.
	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: files,
		ProtoFile:      fdProtos,
	}

	plugin, err := protogen.Options{}.New(req)
	if err != nil {
		return nil, fmt.Errorf("protogen plugin creation failed: %w", err)
	}

	root := parser.Parse(plugin)
	r := resolver.New(root)
	if err := r.Resolve(); err != nil {
		return nil, fmt.Errorf("resolve failed: %w", err)
	}

	return root, nil
}

// collectFileDescriptors recursively collects FileDescriptorProtos for the
// given file and all of its transitive imports. Dependencies are added before
// the files that depend on them (topological order), which is required by
// CodeGeneratorRequest.ProtoFile.
func collectFileDescriptors(fd protoreflect.FileDescriptor, seen map[string]bool, out *[]*descriptorpb.FileDescriptorProto) {
	path := fd.Path()
	if seen[path] {
		return
	}
	seen[path] = true

	// Process imports first (dependencies before dependents).
	imports := fd.Imports()
	for i := 0; i < imports.Len(); i++ {
		imp := imports.Get(i)
		collectFileDescriptors(imp.FileDescriptor, seen, out)
	}

	*out = append(*out, protodesc.ToFileDescriptorProto(fd))
}

// FindProtoFiles walks a directory tree and returns the relative paths of all
// .proto files found.
func FindProtoFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".proto") {
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			files = append(files, rel)
		}
		return nil
	})
	return files, err
}
