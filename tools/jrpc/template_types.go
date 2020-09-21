package main

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/jakewright/home-automation/libraries/go/ptr"
	"github.com/jakewright/home-automation/tools/libraries/imports"
)

const packageDirExternal = "def"

type typesDataMessage struct {
	Name   string
	Fields []*typesDataField
}

type typesDataField struct {
	GoName        string
	JSONName      string
	Type          string
	IsMessageType bool // Used to know whether the field can be recursively validated
	Repeated      bool
	Ptr           bool // Ptr is set if the field is not a reference type (slice or map)

	// Field options
	Required bool
	Min      *float64
	Max      *float64
}

type typesData struct {
	PackageName string
	Imports     []*imports.Imp
	Messages    []*typesDataMessage
}

const typesTemplateText = `// Code generated by jrpc. DO NOT EDIT.

package {{ .PackageName }}

{{ if .Imports }}
	import (
		{{- range .Imports }}
			{{ .Alias }} "{{ .Path }}"
		{{- end}}
	)
{{- end }}

{{ range $message := .Messages }}
	// {{ $message.Name }} is defined in the .def file
	type {{ $message.Name }} struct {
		{{- range $field := .Fields }}
			{{ $field.GoName }} {{ if $field.Ptr }}*{{ end }}{{ $field.Type }} ` + "`" + `json:"{{ $field.JSONName }},omitempty"` + "`" + `
		{{- end }}
	}

	{{- range $field := .Fields }}
		// Get{{ $field.GoName }} returns the de-referenced value of {{ $field.GoName }}.
		{{ if $field.Required }} // If the field is nil, the function panics because {{ $field.JSONName }} is marked as required. 
		{{- else }} // The second return value states whether the field was set. {{ end }}
		func (m *{{ $message.Name }}) Get{{ $field.GoName }}() (val {{ $field.Type }}{{ if not $field.Required }}, set bool{{ end }}) {
			if m.{{ $field.GoName }} == nil {
				{{ if $field.Required }} panic("{{ $field.JSONName }} marked as required but was not set. This should have been caught by the validate function.") {{ else }} return {{ end }}
			}

			return {{ if $field.Ptr }}*{{ end }}m.{{ $field.GoName }}{{ if not $field.Required }}, true{{ end }}
		}

		// Set{{ $field.GoName }} sets the value of {{ $field.GoName }}
		func (m *{{ $message.Name }}) Set{{ $field.GoName }}(v {{ $field.Type }}) *{{ $message.Name }} {
			m.{{ $field.GoName }} = {{ if $field.Ptr }}&{{ end }}v
			return m
		}
	{{- end }}

	// Validate returns an error if any of the fields have bad values
	func (m *{{ $message.Name }}) Validate() error {
		{{- range $field := $message.Fields -}}
			{{ if $field.IsMessageType -}}
				{{ if $field.Repeated -}}
					if m.{{ $field.GoName }} != nil {
						for _, r := range m.{{ $field.GoName }} {
							if err := r.Validate(); err != nil {
								return err
							}
						}
					}
				{{ else -}}
					if err := m.{{ $field.GoName }}.Validate(); err != nil {
						return err
					}
				{{ end }}
			{{ end -}}

			{{ if $field.Required -}}
				if m.{{ $field.GoName }} == nil {
					return oops.BadRequest("field '{{ $field.JSONName }}' is required")
				}
			{{ end -}}

			{{ if $field.Min -}}
				if m.{{ $field.GoName }} != nil && *m.{{ $field.GoName }} < {{ $field.Min }} {
					return oops.BadRequest("field '{{ $field.JSONName }}' should be ≥ {{ $field.Min }}")
				}
			{{ end -}}

			{{ if $field.Max -}}
				if m.{{ $field.GoName }} != nil && *m.{{ $field.GoName }} > {{ $field.Max }} {
					return oops.BadRequest("field '{{ $field.JSONName }}' should be ≤ {{ $field.Max }}")
				}
			{{ end -}}
		{{ end -}}

		return nil
	}
{{ end }}

`

type typesGenerator struct {
	baseGenerator
}

func (g *typesGenerator) Template() (*template.Template, error) {
	return template.New("types_template").Parse(typesTemplateText)
}

func (g *typesGenerator) PackageDir() string {
	return packageDirExternal
}

func (g *typesGenerator) Data(im *imports.Manager) (interface{}, error) {
	im.Add("github.com/jakewright/home-automation/libraries/go/oops")

	// util is needed for the color type if any fields have type "rgb"
	im.Add("github.com/jakewright/home-automation/libraries/go/util")

	if len(g.file.Messages) == 0 {
		return nil, nil
	}

	var messages []*typesDataMessage
	for _, m := range g.file.FlatMessages {
		alias, parts := m.Lineage()
		if alias != "" {
			// Ignore any messages that are from imported files
			continue
		}

		name := strings.Join(parts, "_")
		if !reValidGoStructUnderscore.MatchString(name) {
			return nil, fmt.Errorf("invalid message name %s", name)
		}

		fields := make([]*typesDataField, len(m.Fields))
		for i, f := range m.Fields {
			goName, jsonName, err := convertFieldName(f.Name)
			if err != nil {
				return nil, err
			}

			typ, err := resolveTypeName(f.Type, g.file, im)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve type of field %q in message %q: %w", f.Name, m.Name, err)
			}

			/* Parse the field options */

			var required bool
			if v, ok := f.Options["required"].(bool); ok {
				required = v
			}

			var min *float64
			if v, ok := f.Options["min"]; ok {
				switch t := v.(type) {
				case float64:
					if !typ.isFloat() {
						return nil, fmt.Errorf("cannot set float min on non-float field")
					}
					min = &t
				case int64:
					if !typ.isInt() && !typ.isFloat() {
						return nil, fmt.Errorf("cannot set min on non-numeric field")
					}
					min = ptr.Float64(float64(t))
				default:
					return nil, fmt.Errorf("value of min option has invalid type")
				}
			}

			var max *float64
			if v, ok := f.Options["max"]; ok {
				switch t := v.(type) {
				case float64:
					if !typ.isFloat() {
						return nil, fmt.Errorf("cannot set float max on non-float field")
					}
					max = &t
				case int64:
					if !typ.isInt() && !typ.isFloat() {
						return nil, fmt.Errorf("cannot set max on non-numeric field")
					}
					max = ptr.Float64(float64(t))
				default:
					return nil, fmt.Errorf("value of max option has invalid type")
				}
			}

			fields[i] = &typesDataField{
				GoName:        goName,
				JSONName:      jsonName,
				Type:          typ.FullTypeName,
				IsMessageType: typ.IsMessageType,
				Repeated:      typ.Repeated,
				Ptr:           !(f.Type.Map || f.Type.Repeated), // [1]
				Required:      required,
				Min:           min,
				Max:           max,
			}
		}

		messages = append(messages, &typesDataMessage{
			Name:   name,
			Fields: fields,
		})
	}

	return &typesData{
		PackageName: externalPackageName(g.options),
		Imports:     im.Get(),
		Messages:    messages,
	}, nil
}

func (g *typesGenerator) Filename() string {
	return "types.go"
}

// [1] A note on reference types
// An empty array in JSON becomes an empty slice in go when unmarshaled.
// If the JSON field is not set, then the slice in go remains nil. It
// is therefore not necessary for slices to use pointers to determine
// whether or not the value was set in the JSON.
