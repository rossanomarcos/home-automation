package main

import (
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"github.com/jakewright/home-automation/libraries/go/protoparse"
	jrpcproto "github.com/jakewright/home-automation/tools/protoc-gen-jrpc/proto"
)

var routerTemplate *template.Template

func init() {
	var err error
	routerTemplate, err = template.New("router_template").Parse(routerTemplateText)
	if err != nil {
		panic(err)
	}
}

type method struct {
	Name       string
	InputType  string
	OutputType string
	HTTPMethod string
	Path       string
	URL        string
}

type routerData struct {
	PackageName string
	RouterName  string
	Imports     []*protoparse.Import
	Methods     []*method
}

const routerTemplateText = `// Code generated by protoc-gen-jrpc. DO NOT EDIT.

package {{ .PackageName }}

{{ if .Imports }}
	import (
		{{- range .Imports }}
			{{ .Alias }} "{{ .Path }}"
		{{- end }}
	)
{{ end }}

// {{ .RouterName }} wraps router.Router to provide a convenient way to set handlers
type {{ .RouterName }} struct {
	*router.Router
	{{- range .Methods }}
    	{{ .Name }} func(*{{ .InputType }}) (*{{ .OutputType }}, error)
	{{- end }}
}

// NewRouter returns a router that is ready to add handlers to
func NewRouter() *{{ .RouterName }} {
	rr := &{{ .RouterName }}{
		Router: router.New(),
	}

	{{ range .Methods }}
		rr.Router.Handle("{{ .HTTPMethod }}", "{{ .Path }}", func(w http.ResponseWriter, r *http.Request) {
			if rr.{{ .Name }} == nil {
				slog.Panicf("No handler exists for {{ .HTTPMethod }} {{ .URL }}")
			}

			body := &{{ .InputType }}{}
			if err := request.Decode(r, body); err != nil {
				err = errors.Wrap(err, errors.ErrBadRequest, "failed to decode request")
				slog.Error(err)
				response.WriteJSON(w, err)
				return
			}

			if err := body.Validate(); err != nil {
				err = errors.Wrap(err, errors.ErrBadRequest, "failed to validate request")
				slog.Error(err)
				response.WriteJSON(w, err)
				return
			}

			rsp, err := rr.{{ .Name }}(body)
			if err != nil {
				err = errors.WithMessage(err, "failed to handle request")
				slog.Error(err)
				response.WriteJSON(w, err)
				return
			}

			response.WriteJSON(w, rsp)
		})
	{{ end }}

	return rr
}

{{ range .Methods }}
	// Do makes performs the request
	func (m *{{ .InputType }}) Do() (*{{ .OutputType }}, error) {
		req := &rpc.Request{
			Method: "{{ .HTTPMethod }}",
			URL: "{{ .URL }}",
			Body: m,
		}

		rsp := &{{ .OutputType }}{}
		_, err := rpc.Do(req, rsp)
		return rsp, err
	}
{{ end }}
`

func createRouterTemplateData(file *protoparse.File, service *protoparse.Service) (*routerData, error) {
	imports := append(file.Imports,
		&protoparse.Import{Alias: "", Path: "net/http"},
		&protoparse.Import{Alias: "", Path: "github.com/jakewright/home-automation/libraries/go/errors"},
		&protoparse.Import{Alias: "", Path: "github.com/jakewright/home-automation/libraries/go/request"},
		&protoparse.Import{Alias: "", Path: "github.com/jakewright/home-automation/libraries/go/response"},
		&protoparse.Import{Alias: "", Path: "github.com/jakewright/home-automation/libraries/go/router"},
		&protoparse.Import{Alias: "", Path: "github.com/jakewright/home-automation/libraries/go/rpc"},
		&protoparse.Import{Alias: "", Path: "github.com/jakewright/home-automation/libraries/go/slog"},
	)

	// Get the service options
	opts, err := service.GetExtension(jrpcproto.E_Router)
	if err != nil {
		return nil, err
	}
	router := opts.(*jrpcproto.Router)

	methods := make([]*method, len(service.Methods))
	for i, m := range service.Methods {
		// Get the handler options
		opts, err := m.GetExtension(jrpcproto.E_Handler)
		if err != nil {
			return nil, err
		}
		handler := opts.(*jrpcproto.Handler)

		methods[i] = &method{
			Name:       m.Name,
			InputType:  qualifyTypeName(m.InputType, file),
			OutputType: qualifyTypeName(m.OutputType, file),
			HTTPMethod: handler.Method,
			Path:       handler.Path,
			URL:        router.Name + handler.Path,
		}
	}

	routerName, err := generateRouterName(service.Name)
	if err != nil {
		return nil, err
	}

	return &routerData{
		PackageName: file.GoPackage,
		Imports:     imports,
		RouterName:  routerName,
		Methods:     methods,
	}, nil
}

func generateRouterName(serviceName string) (string, error) {
	err := fmt.Errorf("service name should be alphanumeric camelcase ending with \"Service\"")

	r := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]*Service$`)
	if ok := r.MatchString(serviceName); !ok {
		return "", err
	}

	return strings.Title(strings.TrimSuffix(serviceName, "Service")) + "Router", nil
}

func qualifyTypeName(message *protoparse.Message, file *protoparse.File) string {
	name := message.GoTypeName
	// Prepend the type name with the package name if different
	// from the package name of the file we're generating
	if message.File.GoPackage != file.GoPackage {
		name = message.File.GoPackage + "." + name
	}
	return name
}
