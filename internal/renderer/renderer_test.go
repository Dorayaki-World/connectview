package renderer_test

import (
	"strings"
	"testing"

	"github.com/Dorayaki-World/connectview/internal/ir"
	"github.com/Dorayaki-World/connectview/internal/renderer"
)

func greetServiceIR() *ir.Root {
	return &ir.Root{
		Services: []*ir.Service{
			{
				Name:            "GreetService",
				FullName:        "connectrpc.greet.v1.GreetService",
				Comment:         "GreetService provides greeting functionality.",
				ConnectBasePath: "/connectrpc.greet.v1.GreetService/",
				RPCs: []*ir.RPC{
					{
						Name:        "Greet",
						Comment:     "Greet sends a greeting to the named subject.",
						ConnectPath: "/connectrpc.greet.v1.GreetService/Greet",
						HTTPMethod:  "POST",
						Request: &ir.MessageRef{
							TypeName: ".connectrpc.greet.v1.GreetRequest",
							Resolved: &ir.Message{
								Name:     "GreetRequest",
								FullName: ".connectrpc.greet.v1.GreetRequest",
								Comment:  "GreetRequest contains the subject to greet.",
								Fields: []*ir.Field{
									{Name: "name", Type: ir.FieldTypeString, Label: ir.FieldLabelOptional, Comment: "The subject to greet."},
									{Name: "locale", Type: ir.FieldTypeString, Label: ir.FieldLabelOptional, Comment: "The locale for the greeting. Optional.", IsOptional: true},
								},
							},
						},
						Response: &ir.MessageRef{
							TypeName: ".connectrpc.greet.v1.GreetResponse",
							Resolved: &ir.Message{
								Name:     "GreetResponse",
								FullName: ".connectrpc.greet.v1.GreetResponse",
								Comment:  "GreetResponse contains the greeting message.",
								Fields: []*ir.Field{
									{Name: "greeting", Type: ir.FieldTypeString, Label: ir.FieldLabelOptional, Comment: "The greeting."},
								},
							},
						},
					},
				},
			},
		},
		Messages: make(map[string]*ir.Message),
		Enums:    make(map[string]*ir.Enum),
	}
}

func renderHTML(t *testing.T) string {
	t.Helper()
	root := greetServiceIR()
	r := renderer.New()
	html, err := r.Render(root)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	return html
}

func TestRenderer_ValidHTML(t *testing.T) {
	html := renderHTML(t)
	if !strings.HasPrefix(html, "<!DOCTYPE html>") {
		t.Error("HTML does not start with <!DOCTYPE html>")
	}
	if !strings.HasSuffix(strings.TrimSpace(html), "</html>") {
		t.Error("HTML does not end with </html>")
	}
}

func TestRenderer_ContainsServiceComment(t *testing.T) {
	html := renderHTML(t)
	if !strings.Contains(html, "GreetService provides greeting functionality.") {
		t.Error("HTML does not contain service comment")
	}
}

func TestRenderer_ContainsConnectPath(t *testing.T) {
	html := renderHTML(t)
	if !strings.Contains(html, "/connectrpc.greet.v1.GreetService/Greet") {
		t.Error("HTML does not contain ConnectRPC path")
	}
}

func TestRenderer_ContainsHTTPMethod(t *testing.T) {
	html := renderHTML(t)
	if !strings.Contains(html, "POST") {
		t.Error("HTML does not contain HTTP method POST")
	}
}

func TestRenderer_ContainsEmbeddedSchema(t *testing.T) {
	html := renderHTML(t)
	if !strings.Contains(html, "__CONNECTVIEW_SCHEMA__") {
		t.Error("HTML does not contain embedded schema JSON")
	}
	if !strings.Contains(html, "GreetService") {
		t.Error("HTML does not contain service name in embedded schema")
	}
}

func TestRenderer_ContainsFieldNames(t *testing.T) {
	html := renderHTML(t)
	for _, field := range []string{"name", "locale", "greeting"} {
		if !strings.Contains(html, `"`+field+`"`) {
			t.Errorf("HTML does not contain field name %q in schema JSON", field)
		}
	}
}

func TestRenderer_SelfContained(t *testing.T) {
	html := renderHTML(t)
	for _, cdn := range []string{"cdn.", "googleapis.com", "cdnjs.com", "unpkg.com", "jsdelivr.net"} {
		if strings.Contains(html, cdn) {
			t.Errorf("HTML contains external CDN reference: %q", cdn)
		}
	}
}
