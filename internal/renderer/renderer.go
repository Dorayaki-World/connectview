package renderer

import (
	"bytes"
	"encoding/json"
	"html/template"

	"github.com/Dorayaki-World/connectview/internal/ir"
)

// Renderer generates a self-contained HTML file from the resolved IR.
type Renderer struct{}

func New() *Renderer {
	return &Renderer{}
}

// templateData is passed to the HTML template.
type templateData struct {
	SchemaJSON template.JS
	CSS        template.CSS
	JS         template.JS
}

// schemaJSON is the JSON-serializable representation of the IR.
type schemaJSON struct {
	Services []*serviceJSON `json:"services"`
}

type serviceJSON struct {
	Name            string     `json:"name"`
	FullName        string     `json:"fullName"`
	File            string     `json:"file"`
	Comment         string     `json:"comment"`
	ConnectBasePath string     `json:"connectBasePath"`
	RPCs            []*rpcJSON `json:"rpcs"`
}

type rpcJSON struct {
	Name           string       `json:"name"`
	Comment        string       `json:"comment"`
	ConnectPath    string       `json:"connectPath"`
	HTTPMethod     string       `json:"httpMethod"`
	ClientStreaming bool        `json:"clientStreaming"`
	ServerStreaming bool        `json:"serverStreaming"`
	Request        *messageRefJSON `json:"request"`
	Response       *messageRefJSON `json:"response"`
}

type messageRefJSON struct {
	TypeName string       `json:"typeName"`
	Resolved *messageJSON `json:"resolved,omitempty"`
}

type messageJSON struct {
	Name    string       `json:"name"`
	FullName string      `json:"fullName"`
	Comment string       `json:"comment"`
	Fields  []*fieldJSON `json:"fields"`
}

type fieldJSON struct {
	Name             string       `json:"name"`
	Number           int32        `json:"number"`
	Type             int32        `json:"type"`
	TypeName         string       `json:"typeName"`
	Label            int32        `json:"label"`
	Comment          string       `json:"comment"`
	OneofName        string       `json:"oneofName"`
	IsOptional       bool         `json:"isOptional"`
	IsMap            bool         `json:"isMap"`
	MapKeyType       int32        `json:"mapKeyType,omitempty"`
	MapValueType     int32        `json:"mapValueType,omitempty"`
	MapValueTypeName string       `json:"mapValueTypeName,omitempty"`
	IsRecursive      bool         `json:"isRecursive"`
	ResolvedMessage  *messageJSON `json:"resolvedMessage,omitempty"`
	ResolvedEnum     *enumJSON    `json:"resolvedEnum,omitempty"`
}

type enumJSON struct {
	Name     string           `json:"name"`
	FullName string           `json:"fullName"`
	Comment  string           `json:"comment"`
	Values   []*enumValueJSON `json:"values"`
}

type enumValueJSON struct {
	Name    string `json:"name"`
	Number  int32  `json:"number"`
	Comment string `json:"comment"`
}

// Render produces a self-contained HTML string from the resolved IR.
func (r *Renderer) Render(root *ir.Root) (string, error) {
	schema := buildSchema(root)

	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return "", err
	}

	css, err := assets.ReadFile("assets/style.css")
	if err != nil {
		return "", err
	}

	js, err := assets.ReadFile("assets/app.js")
	if err != nil {
		return "", err
	}

	tmplBytes, err := assets.ReadFile("assets/index.html.tmpl")
	if err != nil {
		return "", err
	}

	tmpl, err := template.New("index").Parse(string(tmplBytes))
	if err != nil {
		return "", err
	}

	data := templateData{
		SchemaJSON: template.JS(schemaBytes),
		CSS:        template.CSS(css),
		JS:         template.JS(js),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func buildSchema(root *ir.Root) *schemaJSON {
	schema := &schemaJSON{}
	for _, svc := range root.Services {
		schema.Services = append(schema.Services, convertService(svc))
	}
	return schema
}

func convertService(svc *ir.Service) *serviceJSON {
	s := &serviceJSON{
		Name:            svc.Name,
		FullName:        svc.FullName,
		File:            svc.File,
		Comment:         svc.Comment,
		ConnectBasePath: svc.ConnectBasePath,
	}
	for _, rpc := range svc.RPCs {
		s.RPCs = append(s.RPCs, convertRPC(rpc))
	}
	return s
}

func convertRPC(rpc *ir.RPC) *rpcJSON {
	return &rpcJSON{
		Name:           rpc.Name,
		Comment:        rpc.Comment,
		ConnectPath:    rpc.ConnectPath,
		HTTPMethod:     rpc.HTTPMethod,
		ClientStreaming: rpc.ClientStreaming,
		ServerStreaming: rpc.ServerStreaming,
		Request:        convertMessageRef(rpc.Request),
		Response:       convertMessageRef(rpc.Response),
	}
}

func convertMessageRef(ref *ir.MessageRef) *messageRefJSON {
	if ref == nil {
		return nil
	}
	r := &messageRefJSON{
		TypeName: ref.TypeName,
	}
	if ref.Resolved != nil {
		r.Resolved = convertMessage(ref.Resolved, nil)
	}
	return r
}

// convertMessage converts an ir.Message to JSON, tracking visited messages to handle recursion.
func convertMessage(msg *ir.Message, visited map[string]bool) *messageJSON {
	if msg == nil {
		return nil
	}
	if visited == nil {
		visited = make(map[string]bool)
	}
	m := &messageJSON{
		Name:     msg.Name,
		FullName: msg.FullName,
		Comment:  msg.Comment,
	}
	for _, f := range msg.Fields {
		m.Fields = append(m.Fields, convertField(f, visited))
	}
	return m
}

func convertField(f *ir.Field, visited map[string]bool) *fieldJSON {
	fj := &fieldJSON{
		Name:             f.Name,
		Number:           f.Number,
		Type:             int32(f.Type),
		TypeName:         f.TypeName,
		Label:            int32(f.Label),
		Comment:          f.Comment,
		OneofName:        f.OneofName,
		IsOptional:       f.IsOptional,
		IsMap:            f.IsMap,
		MapKeyType:       int32(f.MapKeyType),
		MapValueType:     int32(f.MapValueType),
		MapValueTypeName: f.MapValueTypeName,
		IsRecursive:      f.IsRecursive,
	}

	if f.ResolvedEnum != nil {
		fj.ResolvedEnum = convertEnum(f.ResolvedEnum)
	}

	if f.ResolvedMessage != nil {
		if f.IsRecursive {
			// For recursive fields, include type info but don't expand nested fields
			fj.ResolvedMessage = &messageJSON{
				Name:     f.ResolvedMessage.Name,
				FullName: f.ResolvedMessage.FullName,
				Comment:  f.ResolvedMessage.Comment,
			}
		} else {
			fj.ResolvedMessage = convertMessage(f.ResolvedMessage, visited)
		}
	}

	return fj
}

func convertEnum(e *ir.Enum) *enumJSON {
	ej := &enumJSON{
		Name:     e.Name,
		FullName: e.FullName,
		Comment:  e.Comment,
	}
	for _, v := range e.Values {
		ej.Values = append(ej.Values, &enumValueJSON{
			Name:    v.Name,
			Number:  v.Number,
			Comment: v.Comment,
		})
	}
	return ej
}
