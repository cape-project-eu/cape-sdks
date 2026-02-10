//go:build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	"go.yaml.in/yaml/v4"

	"cape-project.eu/sdk-generator/provider/pulumi/internal/codegen"
)

const SchemasDir = "../../../ext/secapi/spec/schemas"
const PulumiControlResourceFile = "../pulumi.gen.yaml"
const SchemasImportPath = "cape-project.eu/sdk-generator/provider/pulumi/internal/schemas"

var resourceTemplate = codegen.ReadTemplate("resource", "codegen/resource.tmpl")
var createTemplate = codegen.ReadTemplate("create", "codegen/create.tmpl")
var readTemplate = codegen.ReadTemplate("read", "codegen/read.tmpl")
var updateTemplate = codegen.ReadTemplate("update", "codegen/update.tmpl")
var deleteTemplate = codegen.ReadTemplate("delete", "codegen/delete.tmpl")
var apiTemplate = codegen.ReadTemplate("api", "codegen/api.tmpl")
var converterTemplate = codegen.ReadTemplate("converter", "codegen/converter.tmpl")

func main() {
	cwd, _ := os.Getwd()
	schemaRoot := SchemasDir
	if !filepath.IsAbs(schemaRoot) {
		schemaRoot = filepath.Join(cwd, schemaRoot)
	}
	controlPath := PulumiControlResourceFile
	if !filepath.IsAbs(controlPath) {
		controlPath = filepath.Join(cwd, controlPath)
	}

	models := codegen.GetModelsForPath(schemaRoot)
	resolver := codegen.NewSchemaResolver(models)
	resources, err := codegen.LoadControlResources(controlPath)
	if err != nil {
		fmt.Printf("error reading control resources: %v\n", err)
		return
	}

	resourceNames := make([]string, 0, len(resources))
	for name := range resources {
		resourceNames = append(resourceNames, name)
	}
	sort.Strings(resourceNames)

	for _, name := range resourceNames {
		spec := resources[name]
		if spec.Package == "" {
			continue
		}
		outDir := filepath.Join(cwd, spec.Package)
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			fmt.Printf("error creating output dir %s: %v\n", outDir, err)
			continue
		}
		def := buildResourceDef(name, spec, resolver)

		fileName := fmt.Sprintf("%s.gen.go", strings.ToLower(name))
		outPath := filepath.Join(outDir, fileName)
		writeTemplate(outPath, def, resourceTemplate)

		fileName = fmt.Sprintf("%s.create.gen.go", strings.ToLower(name))
		outPath = filepath.Join(outDir, fileName)
		writeTemplate(outPath, def, createTemplate)

		fileName = fmt.Sprintf("%s.read.gen.go", strings.ToLower(name))
		outPath = filepath.Join(outDir, fileName)
		writeTemplate(outPath, def, readTemplate)

		fileName = fmt.Sprintf("%s.update.gen.go", strings.ToLower(name))
		outPath = filepath.Join(outDir, fileName)
		writeTemplate(outPath, def, updateTemplate)

		fileName = fmt.Sprintf("%s.delete.gen.go", strings.ToLower(name))
		outPath = filepath.Join(outDir, fileName)
		writeTemplate(outPath, def, deleteTemplate)

		fileName = fmt.Sprintf("%s.api.gen.go", strings.ToLower(name))
		outPath = filepath.Join(outDir, fileName)
		writeTemplate(outPath, def, apiTemplate)

		if !def.WithCustomGenerators {
			fileName = fmt.Sprintf("%s.converter.gen.go", strings.ToLower(name))
			outPath = filepath.Join(outDir, fileName)
			writeTemplate(outPath, def, converterTemplate)
		}
	}
}

type resourceField struct {
	Name       string
	Type       string
	Tag        string
	Desc       string
	Default    string
	Optional   bool
	HasDefault bool
	Annotate   bool
}

type resourceDef struct {
	Name                 string
	Package              string
	APIPackage           string
	APIPackageID         string
	WithoutWorkspace     bool
	WithCustomGenerators bool
	Inputs               []resourceField
	Outputs              []resourceField
	ResourceDesc         string
	ArgsAnnotateLines    []string
	StateAnnotateLines   []string
}

func buildResourceDef(name string, spec codegen.ControlResourceSpec, resolver *codegen.SchemaResolver) resourceDef {
	inputs := make([]resourceField, 0, len(spec.Input))
	for _, input := range spec.Input {
		fieldName := input.Name
		hasOverride := input.Type != "" || input.Description != "" || input.Default != nil
		typeName := input.Type
		if typeName == "" {
			typeName = resolveControlType(name, fieldName, resolver)
		}

		optional := false
		if hasOverride && input.Type != "" {
			optional = strings.Contains(input.Type, "*")
		} else {
			optional = !isRequiredInput(fieldName)
		}

		fieldType := qualifySchemaType(typeName)
		if hasOverride && input.Type != "" {
			fieldType = input.Type
		} else if optional {
			fieldType = "*" + fieldType
		}

		tag := buildControlTag(fieldName, optional)

		desc := ""
		defVal := ""
		hasDef := false
		annotate := true
		if hasOverride {
			if input.Description != "" {
				desc = codegen.NormalizeDescription(input.Description)
				defVal, hasDef = defaultLiteralFromNode(input.Type, input.Default)
			} else {
				annotate = false
			}
		} else {
			desc = propertyDescription(name, fieldName, resolver)
			defVal, hasDef = propertyDefaultLiteral(name, fieldName, resolver)
		}
		inputs = append(inputs, resourceField{
			Name:       fieldName,
			Type:       fieldType,
			Tag:        tag,
			Desc:       desc,
			Default:    defVal,
			HasDefault: hasDef,
			Annotate:   annotate,
		})
	}

	outputs := make([]resourceField, 0, len(spec.Output))
	for _, output := range spec.Output {
		fieldName := output.Name
		hasOverride := output.Type != "" || output.Description != "" || output.Default != nil
		typeName := output.Type
		if typeName == "" {
			typeName = resolveControlType(name, fieldName, resolver)
		}

		optional := false
		if hasOverride && output.Type != "" {
			optional = strings.Contains(output.Type, "*")
		}

		fieldType := qualifySchemaType(typeName)
		if hasOverride && output.Type != "" {
			fieldType = output.Type
		}

		tag := buildControlTag(fieldName, optional)

		desc := ""
		defVal := ""
		hasDef := false
		annotate := true
		if hasOverride {
			if output.Description != "" {
				desc = codegen.NormalizeDescription(output.Description)
				defVal, hasDef = defaultLiteralFromNode(output.Type, output.Default)
			} else {
				annotate = false
			}
		} else {
			desc = propertyDescription(name, fieldName, resolver)
			defVal, hasDef = propertyDefaultLiteral(name, fieldName, resolver)
		}
		outputs = append(outputs, resourceField{
			Name:       fieldName,
			Type:       fieldType,
			Tag:        tag,
			Desc:       desc,
			Default:    defVal,
			Optional:   optional,
			HasDefault: hasDef,
			Annotate:   annotate,
		})
	}

	argsAnnotate := buildAnnotateLines(inputs)
	stateAnnotate := buildAnnotateLines(outputs)
	resourceDesc := schemaDescriptionString(name, resolver)

	s := strings.Split(spec.APIPackage, "/")
	return resourceDef{
		Name:                 name,
		Package:              spec.Package,
		APIPackage:           spec.APIPackage,
		APIPackageID:         s[len(s)-1],
		WithoutWorkspace:     spec.WithoutWorkspace,
		WithCustomGenerators: spec.WithCustomGenerators,
		Inputs:               inputs,
		Outputs:              outputs,
		ResourceDesc:         resourceDesc,
		ArgsAnnotateLines:    argsAnnotate,
		StateAnnotateLines:   stateAnnotate,
	}
}

func writeTemplate(outPath string, def resourceDef, tmpl *template.Template) {
	outFile, err := os.Create(outPath)
	if err != nil {
		println(fmt.Errorf("Error creating/opening file: %s", err))
	}
	defer func() {
		_ = outFile.Close()
	}()
	data := struct {
		resourceDef
		SchemasImport string
	}{
		resourceDef:   def,
		SchemasImport: SchemasImportPath,
	}
	err = tmpl.Execute(outFile, data)
	if err != nil {
		println(fmt.Errorf("Error executing template: %s", err))
	}
}

func resolveControlType(resourceName, fieldName string, resolver *codegen.SchemaResolver) string {
	propType := lookupResourcePropertyType(resourceName, fieldName, resolver)
	if propType != "" {
		return propType
	}
	if resolver != nil && resolver.Lookup(fieldName) != nil {
		return fieldName
	}
	candidate := resourceName + fieldName
	if resolver != nil && resolver.Lookup(candidate) != nil {
		return candidate
	}
	return fieldName
}

func lookupResourcePropertyType(resourceName, fieldName string, resolver *codegen.SchemaResolver) string {
	if resolver == nil {
		return ""
	}
	resourceSchema := resolver.Lookup(resourceName)
	if resourceSchema == nil {
		return ""
	}
	schema := resourceSchema.Schema()
	if schema == nil {
		return ""
	}
	propName := codegen.LowerCamel(fieldName)
	propSchema := findPropertySchema(schema, propName, resolver)
	if propSchema == nil {
		return ""
	}
	if propSchema.IsReference() {
		return codegen.RefToSchemaName(propSchema.GetReference())
	}
	prop := propSchema.Schema()
	if prop == nil {
		return ""
	}
	if len(prop.AllOf) == 1 && len(prop.AnyOf) == 0 && len(prop.OneOf) == 0 {
		if prop.AllOf[0] != nil && prop.AllOf[0].IsReference() {
			return codegen.RefToSchemaName(prop.AllOf[0].GetReference())
		}
	}
	return ""
}

func findPropertySchema(schema *base.Schema, name string, resolver *codegen.SchemaResolver) *base.SchemaProxy {
	if schema == nil {
		return nil
	}
	if schema.Properties != nil {
		if prop, ok := schema.Properties.Get(name); ok {
			return prop
		}
	}
	for _, allOf := range schema.AllOf {
		if allOf == nil {
			continue
		}
		allOfSchema := allOf.Schema()
		if allOfSchema == nil {
			continue
		}
		if allOfSchema.Properties != nil {
			if prop, ok := allOfSchema.Properties.Get(name); ok {
				return prop
			}
		}
	}
	return nil
}

func qualifySchemaType(typeName string) string {
	if typeName == "" {
		return "any"
	}
	return "schemas." + typeName
}

func isRequiredInput(name string) bool {
	return name == "Spec"
}

func buildControlTag(name string, optional bool) string {
	tagName := codegen.LowerCamel(name)
	if optional {
		return tagName + ",optional"
	}
	return tagName
}

func buildAnnotateLines(fields []resourceField) []string {
	lines := make([]string, 0, len(fields)*2)
	for _, field := range fields {
		if !field.Annotate {
			continue
		}
		if field.Desc != "" {
			lines = append(lines, fmt.Sprintf("a.Describe(&dto.%s, %q)", field.Name, field.Desc))
		}
		if field.HasDefault {
			lines = append(lines, fmt.Sprintf("a.SetDefault(&dto.%s, %s)", field.Name, field.Default))
		}
		// annotTarget := "dto." + field.Name
		// if !strings.HasPrefix(field.Type, "*") {
		// 	annotTarget = "&dto." + field.Name
		// }
		// lines = append(lines, fmt.Sprintf("utils.AnnotateIfPossible(a, %s)", annotTarget))
	}
	return lines
}

func schemaDescriptionString(name string, resolver *codegen.SchemaResolver) string {
	if resolver == nil {
		return ""
	}
	ref := resolver.Lookup(name)
	if ref == nil {
		return ""
	}
	if schema := ref.Schema(); schema != nil {
		desc := codegen.NormalizeDescription(schema.Description)
		if desc == "" {
			return ""
		}
		return fmt.Sprintf("%q", desc)
	}
	return ""
}

func propertyDescription(resourceName, fieldName string, resolver *codegen.SchemaResolver) string {
	prop := lookupResourceProperty(resourceName, fieldName, resolver)
	if prop == nil {
		return ""
	}
	return codegen.NormalizeDescription(schemaDescription(prop, resolver))
}

func propertyDefaultLiteral(resourceName, fieldName string, resolver *codegen.SchemaResolver) (string, bool) {
	prop := lookupResourceProperty(resourceName, fieldName, resolver)
	if prop == nil {
		return "", false
	}
	return codegen.DefaultValueLiteral(prop)
}

func lookupResourceProperty(resourceName, fieldName string, resolver *codegen.SchemaResolver) *base.SchemaProxy {
	if resolver == nil {
		return nil
	}
	resourceSchema := resolver.Lookup(resourceName)
	if resourceSchema == nil {
		return nil
	}
	schema := resourceSchema.Schema()
	if schema == nil {
		return nil
	}
	propName := codegen.LowerCamel(fieldName)
	return findPropertySchema(schema, propName, resolver)
}

func schemaDescription(schemaProxy *base.SchemaProxy, resolver *codegen.SchemaResolver) string {
	if schemaProxy == nil {
		return ""
	}
	if schemaProxy.IsReference() {
		refName := codegen.RefToSchemaName(schemaProxy.GetReference())
		if resolver == nil {
			return ""
		}
		refSchema := resolver.Lookup(refName)
		if refSchema == nil {
			return ""
		}
		if schema := refSchema.Schema(); schema != nil && schema.Description != "" {
			return schema.Description
		}
		return ""
	}
	schema := schemaProxy.Schema()
	if schema == nil {
		return ""
	}
	if schema.Description != "" {
		return schema.Description
	}
	if len(schema.AllOf) == 1 && schema.AllOf[0] != nil && schema.AllOf[0].IsReference() {
		refName := codegen.RefToSchemaName(schema.AllOf[0].GetReference())
		if resolver == nil {
			return ""
		}
		refSchema := resolver.Lookup(refName)
		if refSchema == nil {
			return ""
		}
		if refSch := refSchema.Schema(); refSch != nil {
			return refSch.Description
		}
	}
	return ""
}

func defaultLiteralFromNode(typeName string, node *yaml.Node) (string, bool) {
	if node == nil {
		return "", false
	}
	if node.Kind == yaml.MappingNode || node.Kind == yaml.SequenceNode {
		return "", false
	}
	baseType := strings.TrimPrefix(typeName, "*")
	switch baseType {
	case "string":
		return fmt.Sprintf("%q", node.Value), true
	case "int", "int64", "float32", "float64", "bool":
		if node.Tag == "!!str" {
			return fmt.Sprintf("%q", node.Value), true
		}
		return node.Value, true
	default:
		if node.Tag == "!!str" {
			return fmt.Sprintf("%q", node.Value), true
		}
		return node.Value, true
	}
}
