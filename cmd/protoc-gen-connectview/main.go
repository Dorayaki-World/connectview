package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/Dorayaki-World/connectview/internal/compiler"
	"github.com/Dorayaki-World/connectview/internal/parser"
	"github.com/Dorayaki-World/connectview/internal/renderer"
	"github.com/Dorayaki-World/connectview/internal/resolver"
	"github.com/Dorayaki-World/connectview/internal/server"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/pluginpb"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		runServe(os.Args[2:])
		return
	}
	runGenerate()
}

func runGenerate() {
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

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	protoDir := fs.String("proto", "", "proto file root directory (required)")
	target := fs.String("target", "", "ConnectRPC target URL (required)")
	port := fs.Int("port", 9000, "listen port")
	var importPaths multiString
	fs.Var(&importPaths, "I", "additional import paths (can be specified multiple times)")
	fs.Parse(args)

	if *protoDir == "" || *target == "" {
		fmt.Fprintln(os.Stderr, "Usage: connectview serve --proto DIR --target URL [--port PORT] [-I PATH]...")
		fs.PrintDefaults()
		os.Exit(1)
	}

	allImportPaths := append([]string{*protoDir}, importPaths...)

	files, err := compiler.FindProtoFiles(*protoDir)
	if err != nil {
		log.Fatalf("failed to find proto files: %v", err)
	}
	if len(files) == 0 {
		log.Fatalf("no .proto files found in %s", *protoDir)
	}

	root, err := compiler.Compile(allImportPaths, files)
	if err != nil {
		log.Fatalf("initial compilation failed: %v", err)
	}

	srv := server.New(*target, root)

	watcher, err := server.NewWatcher(*protoDir, func() {
		log.Println("proto files changed, recompiling...")
		newFiles, err := compiler.FindProtoFiles(*protoDir)
		if err != nil {
			log.Printf("find proto files: %v", err)
			return
		}
		newRoot, err := compiler.Compile(allImportPaths, newFiles)
		if err != nil {
			log.Printf("recompilation failed: %v", err)
			return
		}
		srv.UpdateSchema(newRoot)
		log.Println("schema updated")
	})
	if err != nil {
		log.Fatalf("failed to start watcher: %v", err)
	}
	defer watcher.Close()
	go watcher.Run()

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("connectview serve listening on http://localhost%s", addr)
	log.Printf("  proto:  %s", *protoDir)
	log.Printf("  target: %s", *target)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

type multiString []string

func (m *multiString) String() string { return fmt.Sprint(*m) }
func (m *multiString) Set(val string) error {
	*m = append(*m, val)
	return nil
}
