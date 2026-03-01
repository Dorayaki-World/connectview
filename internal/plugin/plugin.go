package plugin

import (
	"fmt"
	"io"
	"os"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"
)

// Run reads a CodeGeneratorRequest from stdin, calls the generate function,
// and writes the CodeGeneratorResponse to stdout.
func Run(generate func(*pluginpb.CodeGeneratorRequest) (*pluginpb.CodeGeneratorResponse, error)) {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connectview: failed to read stdin: %v\n", err)
		os.Exit(1)
	}

	var req pluginpb.CodeGeneratorRequest
	if err := proto.Unmarshal(input, &req); err != nil {
		fmt.Fprintf(os.Stderr, "connectview: failed to unmarshal request: %v\n", err)
		os.Exit(1)
	}

	resp, err := generate(&req)
	if err != nil {
		resp = &pluginpb.CodeGeneratorResponse{
			Error: proto.String(err.Error()),
		}
	}

	output, err := proto.Marshal(resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connectview: failed to marshal response: %v\n", err)
		os.Exit(1)
	}

	if _, err := os.Stdout.Write(output); err != nil {
		fmt.Fprintf(os.Stderr, "connectview: failed to write stdout: %v\n", err)
		os.Exit(1)
	}
}
