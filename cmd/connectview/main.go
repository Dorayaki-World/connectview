package main

import (
	"flag"

	"github.com/Dorayaki-World/connectview/internal/parser"
	"github.com/Dorayaki-World/connectview/internal/renderer"
	"github.com/Dorayaki-World/connectview/internal/resolver"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/pluginpb"
)

func main() {
	var flags flag.FlagSet
	protogen.Options{
		ParamFunc: flags.Set,
	}.Run(func(plugin *protogen.Plugin) error {
		plugin.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)

		root := parser.Parse(plugin)

		r := resolver.New(root)
		if err := r.Resolve(); err != nil {
			return err
		}

		html, err := renderer.New().Render(root)
		if err != nil {
			return err
		}

		outFile := plugin.NewGeneratedFile("index.html", "")
		_, err = outFile.Write([]byte(html))
		return err
	})
}
