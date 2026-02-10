//go:build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	"go.yaml.in/yaml/v4"

	"cape-project.eu/sdk-generator/provider/pulumi/internal/codegen"
)

const SchemasDir = "../../../../ext/secapi/spec/schemas"
const PulumiControlResourceFile = "../../pulumi.gen.yaml"

func main() {
	cwd, _ := os.Getwd()
	root := SchemasDir
	if !filepath.IsAbs(root) {
		root = filepath.Join(cwd, root)
	}
	skipSchemas := loadPulumiControlResources(filepath.Join(cwd, PulumiControlResourceFile))

	models := codegen.GetModelsForPath(root)
	resolver := codegen.NewSchemaResolver(models)
	for _, entry := range models {
		if entry.Model == nil || entry.Model.Model.Components == nil || entry.Model.Model.Components.Schemas == nil {
			continue
		}
		for name, schema := range entry.Model.Model.Components.Schemas.FromOldest() {
			if skipSchemas[name] {
				continue
			}
			buildTypes(name, schema, cwd, resolver)
		}
	}
}

func loadPulumiControlResources(path string) map[string]bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]bool{}
	}
	var doc struct {
		Resources map[string]any `yaml:"resources"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		fmt.Printf("error reading pulumi control resources: %v\n", err)
		return map[string]bool{}
	}
	skip := make(map[string]bool, len(doc.Resources))
	for name := range doc.Resources {
		skip[name] = true
	}
	return skip
}

type fieldDef struct {
	Name        string
	GoName      string
	GoType      string
	Optional    bool
	Tag         string
	Description string
	Default     string
	HasDefault  bool
	Annotate    bool
}

type dtoDef struct {
	TypeName      string
	Fields        []fieldDef
	Alias         string
	TypeDef       string
	Enums         []enumValue
	Description   string
	AnnotateLines []string
	HasAnnotate   bool
}

var dtoTemplate = codegen.ReadTemplate("dto", "../codegen/schema.tmpl")

func buildTypes(name string, schemaProxy *base.SchemaProxy, outputDir string, resolver *codegen.SchemaResolver) {
	schema := schemaProxy.Schema()
	if schema == nil {
		if err := schemaProxy.GetBuildError(); err != nil {
			fmt.Printf("error building schema %s: %v\n", name, err)
		}
		return
	}

	if enumDTO, ok := buildEnumDTO(name, schema); ok {
		writeDTOFile(enumDTO, outputDir, name)
		return
	}

	if alias, ok := additionalPropertiesAlias(schema, resolver); ok {
		dto := dtoDef{TypeName: toExportedName(name), Alias: alias}
		writeDTOFile(dto, outputDir, name)
		return
	}

	if isUnionSchema(schema) {
		dto := buildUnionDTO(name, schema, resolver)
		writeDTOFile(dto, outputDir, name)
		return
	}

	helpers := map[string]dtoDef{}
	dto := buildObjectDTO(name, schema, resolver, helpers)
	writeDTOFile(dto, outputDir, name)
	for helperName, helper := range helpers {
		writeDTOFile(helper, outputDir, helperName)
	}
}

func writeDTOFile(dto dtoDef, outputDir, name string) {
	fileName := fileNameForSchema(name)
	outPath := filepath.Join(outputDir, fileName)
	outFile, err := os.Create(outPath)
	if err != nil {
		fmt.Printf("error creating %s: %v\n", outPath, err)
		return
	}
	defer func() {
		_ = outFile.Close()
	}()
	if err := dtoTemplate.Execute(outFile, dto); err != nil {
		fmt.Printf("error writing %s: %v\n", outPath, err)
	}
}

func isUnionSchema(schema *base.Schema) bool {
	return schema != nil && (len(schema.AnyOf) > 0 || len(schema.OneOf) > 0)
}

func isEmptySchema(schema *base.Schema, resolver *codegen.SchemaResolver) (typ string, isEmpty bool) {
	if schema == nil {
		return "", false
	}
	hasProps := schema.Properties != nil && schema.Properties.Len() > 0
	hasAddProps := schema.AdditionalProperties != nil
	isUnion := len(schema.AllOf) > 0 || len(schema.AnyOf) > 0 || len(schema.OneOf) > 0

	if !hasProps && !hasAddProps && !isUnion {
		return goTypeForSchema(schema.ParentProxy, resolver, false), true
	}

	return "", false
}

func isSimpleAliasSchema(schema *base.Schema) bool {
	if schema == nil {
		return false
	}
	if schema.Properties != nil && schema.Properties.Len() > 0 {
		return false
	}
	if schema.AdditionalProperties != nil {
		return false
	}
	if len(schema.AllOf) > 0 || len(schema.AnyOf) > 0 || len(schema.OneOf) > 0 {
		return false
	}
	if len(schema.Type) == 0 {
		return false
	}
	switch schema.Type[0] {
	case "string", "integer", "number", "boolean", "array":
		return true
	default:
		return false
	}
}

func buildObjectDTO(name string, schema *base.Schema, resolver *codegen.SchemaResolver, helpers map[string]dtoDef) dtoDef {
	if enumDTO, ok := buildEnumDTO(name, schema); ok {
		return enumDTO
	}

	if alias, ok := additionalPropertiesAlias(schema, resolver); ok {
		return dtoDef{TypeName: toExportedName(name), Alias: alias}
	}

	if typ, isEmpty := isEmptySchema(schema, resolver); isEmpty {
		desc := codegen.NormalizeDescription(schema.Description)
		if desc != "" {
			dto := dtoDef{
				TypeName:    toExportedName(name),
				TypeDef:     typ,
				Description: desc,
				HasAnnotate: true,
			}
			dto.AnnotateLines = buildAnnotateLines(dto.Description, nil)
			return dto
		}
		return dtoDef{TypeName: toExportedName(name), Alias: typ}
	}

	required := requiredSet(schema.Required)
	fields := map[string]*fieldDef{}
	order := make([]string, 0)
	desc := codegen.NormalizeDescription(schema.Description)

	for _, allOf := range schema.AllOf {
		allOfSchema := allOf.Schema()
		if allOfSchema != nil {
			mergeRequired(required, allOfSchema.Required)
		}
	}

	addProperties := func(parentName string, s *base.Schema, optOverride bool) {
		if s == nil || s.Properties == nil {
			if isUnionSchema(s) {
				unionDTO := buildUnionDTO("", s, resolver)
				for _, field := range unionDTO.Fields {
					propName := field.Name
					propOptional := true
					goType := field.GoType
					if existing, ok := fields[propName]; ok {
						existing.Optional = existing.Optional && propOptional
						if !existing.Optional {
							existing.GoType = stripOptionalType(existing.GoType)
						}
						existing.Tag = buildPulumiTag(toLowerCamel(field.GoName), existing.Optional)
						continue
					}
					fields[propName] = &fieldDef{
						Name:     propName,
						GoName:   field.GoName,
						GoType:   goType,
						Optional: propOptional,
						Tag:      field.Tag,
						Annotate: field.Annotate,
					}
					order = append(order, propName)
				}
			}
			return
		}
		for propName, propSchema := range s.Properties.FromOldest() {
			propOptional := optOverride || !required[propName]
			goType := resolvePropertyType(parentName, propName, propSchema, resolver, helpers)
			if propOptional {
				goType = makeOptionalType(goType)
			}
			tag := buildPulumiTag(propName, propOptional)
			propDescription := codegen.NormalizeDescription(schemaDescription(propSchema))
			defaultValue, hasDefault := codegen.DefaultValueLiteral(propSchema)
			annotate := isAnnotatableProperty(propSchema, resolver)

			if existing, ok := fields[propName]; ok {
				existing.Optional = existing.Optional && propOptional
				if !existing.Optional {
					existing.GoType = stripOptionalType(existing.GoType)
				}
				existing.Tag = buildPulumiTag(propName, existing.Optional)
				continue
			}

			fields[propName] = &fieldDef{
				Name:        propName,
				GoName:      toExportedName(propName),
				GoType:      goType,
				Optional:    propOptional,
				Tag:         tag,
				Description: propDescription,
				Default:     defaultValue,
				HasDefault:  hasDefault,
				Annotate:    annotate,
			}
			order = append(order, propName)
		}
	}

	addProperties(name, schema, false)
	for _, allOf := range schema.AllOf {
		addProperties(name, allOf.Schema(), false)
	}

	dto := dtoDef{
		TypeName:    toExportedName(name),
		Fields:      make([]fieldDef, 0, len(order)),
		Description: desc,
		HasAnnotate: true,
	}
	for _, propName := range order {
		dto.Fields = append(dto.Fields, *fields[propName])
	}
	dto.AnnotateLines = buildAnnotateLines(dto.Description, dto.Fields)

	return dto
}

func resolvePropertyType(parentName, propName string, schemaProxy *base.SchemaProxy, resolver *codegen.SchemaResolver, helpers map[string]dtoDef) string {
	if schemaProxy == nil {
		return "any"
	}
	if schemaProxy.IsReference() {
		return goTypeForSchema(schemaProxy, resolver, true)
	}
	schema := schemaProxy.Schema()
	if schema == nil {
		return "any"
	}
	if len(schema.AllOf) == 1 && len(schema.AnyOf) == 0 && len(schema.OneOf) == 0 {
		if schema.AllOf[0] != nil && schema.AllOf[0].IsReference() {
			return goTypeForSchema(schema.AllOf[0], resolver, true)
		}
	}
	if enumDTO, ok := buildEnumDTO(helperTypeName(parentName, propName), schema); ok {
		if _, exists := helpers[enumDTO.TypeName]; !exists {
			helpers[enumDTO.TypeName] = enumDTO
		}
		return enumDTO.TypeName
	}
	if len(schema.AllOf) > 0 || len(schema.AnyOf) > 0 || len(schema.OneOf) > 0 {
		helperName := helperTypeName(parentName, propName)
		if _, exists := helpers[helperName]; !exists {
			if len(schema.AnyOf) > 0 || len(schema.OneOf) > 0 {
				helpers[helperName] = buildUnionDTO(helperName, schema, resolver)
			} else {
				helpers[helperName] = buildObjectDTO(helperName, schema, resolver, helpers)
			}
		}
		return helperName
	}
	return goTypeForSchema(schemaProxy, resolver, true)
}

func buildUnionDTO(name string, schema *base.Schema, resolver *codegen.SchemaResolver) dtoDef {
	fields := make([]fieldDef, 0)
	seen := map[string]int{}
	desc := codegen.NormalizeDescription(schema.Description)

	var variants []*base.SchemaProxy
	if len(schema.AnyOf) > 0 {
		variants = schema.AnyOf
	} else {
		variants = schema.OneOf
	}

	for idx, variant := range variants {
		variantName := unionVariantName(idx, variant)
		goName := toExportedName(variantName)
		if goName == "" {
			goName = fmt.Sprintf("Variant%d", idx+1)
		}
		if count, ok := seen[goName]; ok {
			count++
			seen[goName] = count
			goName = fmt.Sprintf("%s%d", goName, count)
		} else {
			seen[goName] = 1
		}

		goType := goTypeForSchema(variant, resolver, true)
		goType = makeOptionalType(goType)
		tagName := toLowerCamel(goName)
		variantDesc := ""
		if variantSchema := variant.Schema(); variantSchema != nil {
			variantDesc = codegen.NormalizeDescription(variantSchema.Description)
		}
		annotate := isAnnotatableProperty(variant, resolver)
		fields = append(fields, fieldDef{
			Name:        variantName,
			GoName:      goName,
			GoType:      goType,
			Optional:    true,
			Tag:         buildPulumiTag(tagName, true),
			Description: variantDesc,
			Annotate:    annotate,
		})
	}

	dto := dtoDef{
		TypeName:    toExportedName(name),
		Fields:      fields,
		Description: desc,
		HasAnnotate: true,
	}
	dto.AnnotateLines = buildAnnotateLines(dto.Description, dto.Fields)
	return dto
}

func requiredSet(required []string) map[string]bool {
	if len(required) == 0 {
		return map[string]bool{}
	}
	set := make(map[string]bool, len(required))
	for _, name := range required {
		set[name] = true
	}
	return set
}

func mergeRequired(target map[string]bool, required []string) {
	for _, name := range required {
		target[name] = true
	}
}

func goTypeForSchema(schemaProxy *base.SchemaProxy, resolver *codegen.SchemaResolver, allowUnion bool) string {
	if schemaProxy == nil {
		return "any"
	}
	if schemaProxy.IsReference() {
		refName := codegen.RefToSchemaName(schemaProxy.GetReference())
		if resolver != nil {
			if refSchema := resolver.Lookup(refName); refSchema != nil {
				if sch := refSchema.Schema(); sch != nil {
					if enumDTO, ok := buildEnumDTO(refName, sch); ok {
						return enumDTO.TypeName
					}
					if isUnionSchema(sch) {
						if allowUnion {
							return toExportedName(refName)
						}
						return "any"
					}
					if isSimpleAliasSchema(sch) {
						return toExportedName(refName)
					}
					if len(sch.Type) > 0 {
						switch sch.Type[0] {
						case "string":
							return "string"
						case "integer":
							return "int"
						case "number":
							return "float64"
						case "boolean":
							return "bool"
						case "array":
							if sch.Items != nil && sch.Items.IsA() {
								return "[]" + goTypeForSchema(sch.Items.A, resolver, allowUnion)
							}
							return "[]any"
						case "object":
							return toExportedName(refName)
						}
					}
					if sch.Properties != nil {
						return toExportedName(refName)
					}
				}
			}
		}
		return toExportedName(refName)
	}
	schema := schemaProxy.Schema()
	if schema == nil {
		return "any"
	}
	if len(schema.AllOf) == 1 && len(schema.AnyOf) == 0 && len(schema.OneOf) == 0 {
		return goTypeForSchema(schema.AllOf[0], resolver, allowUnion)
	}
	if enumDTO, ok := buildEnumDTO("", schema); ok {
		return enumDTO.Alias
	}
	if isUnionSchema(schema) {
		return "any"
	}
	if len(schema.Type) > 0 {
		switch schema.Type[0] {
		case "string":
			return "string"
		case "integer":
			return "int64"
		case "number":
			return "float64"
		case "boolean":
			return "bool"
		case "array":
			if schema.Items != nil && schema.Items.IsA() {
				return "[]" + goTypeForSchema(schema.Items.A, resolver, allowUnion)
			}
			return "[]any"
		case "object":
			if alias, ok := additionalPropertiesAlias(schema, resolver); ok {
				return alias
			}
			if schema.Properties != nil && schema.Properties.Len() > 0 {
				return "map[string]any"
			}
			return "map[string]any"
		}
	}
	if alias, ok := additionalPropertiesAlias(schema, resolver); ok {
		return alias
	}
	return "any"
}

func makeOptionalType(goType string) string {
	if goType == "" {
		return "*any"
	}
	if strings.HasPrefix(goType, "*") || strings.HasPrefix(goType, "[]") || strings.HasPrefix(goType, "map[") {
		return goType
	}
	return "*" + goType
}

func stripOptionalType(goType string) string {
	if after, ok := strings.CutPrefix(goType, "*"); ok {
		return after
	}
	return goType
}

func buildPulumiTag(name string, optional bool) string {
	tag := "pulumi:\"" + name
	if optional {
		tag += ",optional"
	}
	tag += "\""
	return tag
}

func buildAnnotateLines(desc string, fields []fieldDef) []string {
	lines := make([]string, 0, len(fields)*2+1)
	if desc != "" {
		lines = append(lines, fmt.Sprintf("a.Describe(&dto, %q)", desc))
	}
	for _, field := range fields {
		if field.Description != "" {
			lines = append(lines, fmt.Sprintf("a.Describe(&dto.%s, %q)", field.GoName, field.Description))
		}
		if field.HasDefault {
			lines = append(lines, fmt.Sprintf("a.SetDefault(&dto.%s, %s)", field.GoName, field.Default))
		}
		// if field.Annotate {
		// 	if field.Optional || strings.HasPrefix(field.GoType, "*") {
		// 		lines = append(lines, fmt.Sprintf("if dto.%s != nil { dto.%s.Annotate(a) }", field.GoName, field.GoName))
		// 	} else {
		// 		lines = append(lines, fmt.Sprintf("dto.%s.Annotate(a)", field.GoName))
		// 	}
		// }
	}
	return lines
}

func schemaDescription(schemaProxy *base.SchemaProxy) string {
	if schemaProxy == nil {
		return ""
	}
	if schemaProxy.IsReference() {
		if schema := schemaProxy.Schema(); schema != nil {
			return schema.Description
		}
		return ""
	}
	if schema := schemaProxy.Schema(); schema != nil {
		return schema.Description
	}
	return ""
}

func isAnnotatableProperty(schemaProxy *base.SchemaProxy, resolver *codegen.SchemaResolver) bool {
	if schemaProxy == nil {
		return false
	}
	if schemaProxy.IsReference() {
		return isAnnotatableSchema(schemaProxy, resolver)
	}
	schema := schemaProxy.Schema()
	if schema == nil {
		return false
	}
	if len(schema.AllOf) == 1 && len(schema.AnyOf) == 0 && len(schema.OneOf) == 0 {
		if schema.AllOf[0] != nil && schema.AllOf[0].IsReference() {
			return isAnnotatableSchema(schema.AllOf[0], resolver)
		}
	}
	return isAnnotatableSchema(schemaProxy, resolver)
}

func isAnnotatableSchema(schemaProxy *base.SchemaProxy, resolver *codegen.SchemaResolver) bool {
	if schemaProxy == nil {
		return false
	}
	if schemaProxy.IsReference() {
		refName := codegen.RefToSchemaName(schemaProxy.GetReference())
		if resolver == nil {
			return false
		}
		refSchema := resolver.Lookup(refName)
		if refSchema == nil {
			return false
		}
		schema := refSchema.Schema()
		if schema == nil {
			return false
		}
		if isEnumSchema(schema) {
			return false
		}
		if _, ok := additionalPropertiesAlias(schema, resolver); ok {
			return false
		}
		if isUnionSchema(schema) {
			return true
		}
		if len(schema.AllOf) > 0 {
			return true
		}
		if schema.Properties != nil && schema.Properties.Len() > 0 {
			return true
		}
		return false
	}
	schema := schemaProxy.Schema()
	if schema == nil {
		return false
	}
	if isEnumSchema(schema) {
		return false
	}
	if _, ok := additionalPropertiesAlias(schema, resolver); ok {
		return false
	}
	if isUnionSchema(schema) {
		return true
	}
	if len(schema.AllOf) > 0 {
		return true
	}
	if schema.Properties != nil && schema.Properties.Len() > 0 {
		return true
	}
	return false
}

func isEnumSchema(schema *base.Schema) bool {
	if schema == nil || len(schema.Enum) == 0 {
		return false
	}
	names := enumNamesFromSchema(schema)
	return len(names) == len(schema.Enum)
}

type enumValue struct {
	Name  string
	Value string
}

func buildEnumDTO(name string, schema *base.Schema) (dtoDef, bool) {
	if schema == nil || len(schema.Enum) == 0 {
		return dtoDef{}, false
	}
	enumNames := enumNamesFromSchema(schema)
	if len(enumNames) == 0 || len(enumNames) != len(schema.Enum) {
		return dtoDef{}, false
	}
	baseType := codegen.EnumBaseType(schema)
	typeName := toExportedName(name)
	if typeName == "" && len(enumNames) > 0 {
		typeName = enumTypeFromName(enumNames[0])
	}
	if typeName == "" {
		return dtoDef{}, false
	}

	enums := make([]enumValue, 0, len(schema.Enum))
	for idx, node := range schema.Enum {
		enums = append(enums, enumValue{
			Name:  enumConstName(typeName, enumNames[idx]),
			Value: codegen.EnumValueLiteral(baseType, node),
		})
	}
	return dtoDef{
		TypeName: typeName,
		Alias:    baseType,
		Enums:    enums,
	}, true
}

func enumNamesFromSchema(schema *base.Schema) []string {
	if schema == nil || schema.Extensions == nil {
		return nil
	}
	var node *yaml.Node
	schema.Extensions.FromOldest()(func(key string, value *yaml.Node) bool {
		if key == "x-enumNames" {
			node = value
			return false
		}
		return true
	})
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}
	names := make([]string, 0, len(node.Content))
	for _, item := range node.Content {
		if item != nil {
			names = append(names, item.Value)
		}
	}
	return names
}

func enumTypeFromName(name string) string {
	if name == "" {
		return ""
	}
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] >= 'A' && name[i] <= 'Z' {
			return name[:i]
		}
	}
	return name
}

func enumConstName(typeName, enumName string) string {
	if enumName == "" {
		return typeName
	}
	if typeName == "" {
		return enumName
	}
	if strings.HasPrefix(enumName, typeName) {
		return enumName
	}
	return typeName + enumName
}

func additionalPropertiesAlias(schema *base.Schema, resolver *codegen.SchemaResolver) (string, bool) {
	if schema == nil || schema.AdditionalProperties == nil {
		return "", false
	}
	if len(schema.Type) > 0 && schema.Type[0] != "object" {
		return "", false
	}
	if schema.Properties != nil && schema.Properties.Len() > 0 {
		return "", false
	}
	if schema.AdditionalProperties.IsA() && schema.AdditionalProperties.A != nil {
		valueType := goTypeForSchema(schema.AdditionalProperties.A, resolver, true)
		return "map[string]" + valueType, true
	}
	if schema.AdditionalProperties.IsB() && schema.AdditionalProperties.B {
		return "map[string]any", true
	}
	return "", false
}

func toExportedName(name string) string {
	if name == "" {
		return ""
	}
	var b strings.Builder
	upperNext := true
	wrote := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			if upperNext {
				if r >= 'a' && r <= 'z' {
					r = r - 'a' + 'A'
				}
				upperNext = false
			}
			if !wrote && r >= '0' && r <= '9' {
				b.WriteString("Field")
			}
			b.WriteRune(r)
			wrote = true
			continue
		}
		upperNext = true
	}
	return b.String()
}

func toLowerCamel(name string) string {
	if name == "" {
		return ""
	}
	runes := []rune(name)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func fileNameForSchema(name string) string {
	if name == "" {
		return "unknown.gen.go"
	}
	lower := strings.ToLower(name)
	var b strings.Builder
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		b.WriteRune('_')
	}
	return b.String() + ".gen.go"
}

func unionVariantName(idx int, schemaProxy *base.SchemaProxy) string {
	if schemaProxy == nil {
		return fmt.Sprintf("Variant%d", idx+1)
	}
	if schemaProxy.IsReference() {
		return codegen.RefToSchemaName(schemaProxy.GetReference())
	}
	schema := schemaProxy.Schema()
	if schema != nil {
		if schema.Title != "" {
			return schema.Title
		}
		if len(schema.Type) > 0 {
			return schema.Type[0]
		}
	}
	return fmt.Sprintf("Variant%d", idx+1)
}

func helperTypeName(parentName, propName string) string {
	return toExportedName(parentName) + toExportedName(propName)
}
