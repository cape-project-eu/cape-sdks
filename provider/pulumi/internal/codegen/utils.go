package codegen

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"

	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3high "github.com/pb33f/libopenapi/datamodel/high/v3"
	"go.yaml.in/yaml/v4"
)

func GetModelsForPath(pathDir string) []ModelEntry {
	models := make([]ModelEntry, 0)
	err := filepath.WalkDir(pathDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") {
			model, err := BuildV3Model(path)
			if err != nil {
				fmt.Printf("error building v3Model for %s: %v\n", path, err)
				return nil
			}
			models = append(models, ModelEntry{Path: path, Model: model})
		}
		return nil
	})
	if err != nil {
		fmt.Printf("error walking schema dir: %v\n", err)
		return nil
	}

	return models
}

var tmplFuncMap = template.FuncMap{
	"lower":      strings.ToLower,
	"upper":      strings.ToUpper,
	"pascalCase": PascalCase,
	"camelCase":  func(s string) string { return LowerCamel(PascalCase(s)) },
}

func ReadTemplate(name, filepath string) *template.Template {
	cwd, _ := os.Getwd()
	bytes, _ := os.ReadFile(path.Join(cwd, filepath))

	return template.Must(template.New(name).Funcs(tmplFuncMap).Parse(string(bytes)))
}

type PulumiGenYaml struct {
	SDKVersion string                         `yaml:"sdkVersion"`
	Resources  map[string]ControlResourceSpec `yaml:"resources"`
}

type ControlResourceSpec struct {
	Package              string             `yaml:"package"`
	APIPackage           string             `yaml:"apiPackage"`
	WithoutWorkspace     bool               `yaml:"withoutWorkspace"`
	WithCustomGenerators bool               `yaml:"withCustomGenerators"`
	Input                []ControlFieldSpec `yaml:"input"`
	Output               []ControlFieldSpec `yaml:"output"`
}

type ControlFieldSpec struct {
	Name        string
	Type        string
	Description string
	Default     *yaml.Node
}

func (c *ControlFieldSpec) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		return nil
	}
	switch value.Kind {
	case yaml.ScalarNode:
		c.Name = value.Value
		return nil
	case yaml.MappingNode:
		var raw struct {
			Name        string     `yaml:"name"`
			Type        string     `yaml:"type"`
			Description string     `yaml:"description"`
			Default     *yaml.Node `yaml:"default"`
		}
		if err := value.Decode(&raw); err == nil && (raw.Name != "" || raw.Type != "" || raw.Description != "" || raw.Default != nil) {
			c.Name = raw.Name
			c.Type = raw.Type
			c.Description = raw.Description
			c.Default = raw.Default
			return nil
		}
		if len(value.Content) < 2 {
			return fmt.Errorf("invalid field mapping")
		}
		c.Name = value.Content[0].Value
		c.Type = value.Content[1].Value
		return nil
	default:
		return fmt.Errorf("invalid field type")
	}
}

func GetPulumiGenYaml(path string) (PulumiGenYaml, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PulumiGenYaml{}, err
	}

	var doc PulumiGenYaml
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return PulumiGenYaml{}, err
	}
	if doc.Resources == nil {
		doc.Resources = map[string]ControlResourceSpec{}
	}

	return doc, nil
}

func LoadControlResources(path string) (map[string]ControlResourceSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc PulumiGenYaml
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Resources == nil {
		doc.Resources = map[string]ControlResourceSpec{}
	}
	return doc.Resources, nil
}

type ModelEntry struct {
	Path  string
	Model *libopenapi.DocumentModel[v3high.Document]
}

type SchemaResolver struct {
	Schemas map[string]*base.SchemaProxy
}

func NewSchemaResolver(models []ModelEntry) *SchemaResolver {
	schemas := map[string]*base.SchemaProxy{}
	for _, entry := range models {
		if entry.Model == nil || entry.Model.Model.Components == nil || entry.Model.Model.Components.Schemas == nil {
			continue
		}
		entry.Model.Model.Components.Schemas.FromOldest()(func(name string, schema *base.SchemaProxy) bool {
			schemas[name] = schema
			return true
		})
	}
	return &SchemaResolver{Schemas: schemas}
}

func (r *SchemaResolver) Lookup(name string) *base.SchemaProxy {
	if r == nil {
		return nil
	}
	return r.Schemas[name]
}

func BuildV3Model(file string) (*libopenapi.DocumentModel[v3high.Document], error) {
	schemaFile, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read schema file %s: %w", file, err)
	}

	docBytes := schemaFile
	if !strings.Contains(string(schemaFile), "openapi:") {
		stub := "openapi: 3.0.0\ninfo:\n  title: schema\n  version: 0.0.0\n"
		docBytes = []byte(stub + string(schemaFile))
	}

	openAPIDocument, err := libopenapi.NewDocumentWithConfiguration(docBytes, &datamodel.DocumentConfiguration{
		BypassDocumentCheck: true,
		BasePath:            filepath.Dir(file),
	})
	if err != nil {
		return nil, fmt.Errorf("parse schema file %s: %w", file, err)
	}

	v3Model, err := openAPIDocument.BuildV3Model()
	if err != nil {
		return nil, fmt.Errorf("build v3 model for %s: %w", file, err)
	}
	return v3Model, nil
}

func RefToSchemaName(ref string) string {
	if ref == "" {
		return ""
	}
	if idx := strings.LastIndex(ref, "/"); idx != -1 && idx+1 < len(ref) {
		return ref[idx+1:]
	}
	if idx := strings.LastIndex(ref, "#"); idx != -1 && idx+1 < len(ref) {
		return ref[idx+1:]
	}
	return ref
}

func LowerCamel(name string) string {
	if name == "" {
		return ""
	}
	runes := []rune(name)
	runes[0] = []rune(strings.ToLower(string(runes[0])))[0]
	return string(runes)
}

func PascalCase(s string) string {
	if s == "" {
		return ""
	}

	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
	}

	if ascii {
		out := make([]byte, 0, len(s))
		upperNext := true
		for i := 0; i < len(s); i++ {
			c := s[i]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
				if upperNext && c >= 'a' && c <= 'z' {
					c -= 'a' - 'A'
				}
				out = append(out, c)
				upperNext = false
				continue
			}
			upperNext = true
		}
		return string(out)
	}

	var b strings.Builder
	b.Grow(len(s))
	upperNext := true
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if upperNext {
				r = unicode.ToUpper(r)
			}
			b.WriteRune(r)
			upperNext = false
			continue
		}
		upperNext = true
	}
	return b.String()
}

func NormalizeDescription(desc string) string {
	if desc == "" {
		return ""
	}
	return strings.Join(strings.Fields(desc), " ")
}

func EnumBaseType(schema *base.Schema) string {
	if schema != nil && len(schema.Type) > 0 {
		switch schema.Type[0] {
		case "string":
			return "string"
		case "integer":
			return "int"
		case "number":
			return "float64"
		case "boolean":
			return "bool"
		}
	}
	return "string"
}

func EnumValueLiteral(baseType string, node *yaml.Node) string {
	if node == nil {
		return "\"\""
	}
	switch baseType {
	case "string":
		return fmt.Sprintf("%q", node.Value)
	default:
		if node.Tag == "!!str" {
			return fmt.Sprintf("%q", node.Value)
		}
		return node.Value
	}
}

func DefaultValueLiteral(schemaProxy *base.SchemaProxy) (string, bool) {
	if schemaProxy == nil {
		return "", false
	}
	schema := schemaProxy.Schema()
	if schema == nil || schema.Default == nil {
		return "", false
	}
	if schema.Default.Kind == yaml.MappingNode || schema.Default.Kind == yaml.SequenceNode {
		return "", false
	}
	baseType := EnumBaseType(schema)
	return EnumValueLiteral(baseType, schema.Default), true
}
