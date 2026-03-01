package main

import (
	"github.com/Dorayaki-World/connectview/internal/parser"
	"github.com/Dorayaki-World/connectview/internal/plugin"
	"github.com/Dorayaki-World/connectview/internal/renderer"
	"github.com/Dorayaki-World/connectview/internal/resolver"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"
)

func main() {
	plugin.Run(func(req *pluginpb.CodeGeneratorRequest) (*pluginpb.CodeGeneratorResponse, error) {
		root, err := parser.Parse(req.GetProtoFile())
		if err != nil {
			return nil, err
		}

		r := resolver.New(root)
		if err := r.Resolve(); err != nil {
			return nil, err
		}

		rend := renderer.New()
		html, err := rend.Render(root)
		if err != nil {
			return nil, err
		}

		features := uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		return &pluginpb.CodeGeneratorResponse{
			SupportedFeatures: &features,
			File: []*pluginpb.CodeGeneratorResponse_File{
				{
					Name:    proto.String("index.html"),
					Content: proto.String(html),
				},
			},
		}, nil
	})
}
