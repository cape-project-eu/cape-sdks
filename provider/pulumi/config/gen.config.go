//go:build ignore

package main

import (
	"bytes"
	"fmt"
	"go/format"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"go.yaml.in/yaml/v4"

	"cape-project.eu/sdk-generator/provider/pulumi/internal/codegen"
)

const (
	PulumiControlResourceFile = "../pulumi_control_resources.yaml"
	SecapiSpecDir             = "../../../ext/secapi/spec"
	ConfigTemplatePath        = "../internal/codegen/config.tmpl"
)

var configTemplate = codegen.ReadTemplate("config", ConfigTemplatePath)

type dynamicPrefixField struct {
	FieldName    string
	TagName      string
	Description  string
	DefaultValue string
}

type configTemplateData struct {
	Package       string
	DynamicFields []dynamicPrefixField
}

type openAPISpec struct {
	Servers []struct {
		URL string `yaml:"url"`
	} `yaml:"servers"`
	Info struct {
		Title   string `yaml:"title"`
		Version string `yaml:"version"`
	} `yaml:"info"`
}

func main() {
	cwd, _ := os.Getwd()

	controlPath := PulumiControlResourceFile
	if !filepath.IsAbs(controlPath) {
		controlPath = filepath.Join(cwd, controlPath)
	}
	specRoot := SecapiSpecDir
	if !filepath.IsAbs(specRoot) {
		specRoot = filepath.Join(cwd, specRoot)
	}

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

	dynamicByField := map[string]dynamicPrefixField{}
	for _, name := range resourceNames {
		spec := resources[name]
		if spec.APIPackage == "" {
			continue
		}

		specPath, err := resolveSpecPath(specRoot, spec.APIPackage)
		if err != nil {
			fmt.Printf("error resolving spec path for %s: %v\n", name, err)
			continue
		}
		openapi, err := readSpec(specPath)
		if err != nil {
			fmt.Printf("error reading spec %s: %v\n", specPath, err)
			continue
		}
		if len(openapi.Servers) == 0 || strings.TrimSpace(openapi.Servers[0].URL) == "" {
			fmt.Printf("error reading provider prefix for %s: missing first server url\n", name)
			continue
		}

		prefix, err := providerPrefixFromURL(openapi.Servers[0].URL)
		if err != nil {
			fmt.Printf("error parsing provider prefix for %s: %v\n", name, err)
			continue
		}

		title := strings.TrimSpace(openapi.Info.Title)
		if title == "" {
			title = titleFromAPIPackage(spec.APIPackage)
		}
		version := strings.TrimSpace(openapi.Info.Version)
		fieldBase := toGoIdentifier(title)
		if fieldBase == "" {
			fieldBase = toGoIdentifier(spec.Package)
		}
		if fieldBase == "" {
			fieldBase = toGoIdentifier(name)
		}
		if fieldBase == "" {
			fmt.Printf("error deriving dynamic config field for %s\n", name)
			continue
		}

		suffix := strings.TrimSpace(strings.Join([]string{title, version}, " "))
		if suffix == "" {
			suffix = spec.APIPackage
		}

		fieldName := fieldBase + "ProviderPrefix"
		dynamicByField[fieldName] = dynamicPrefixField{
			FieldName:    fieldName,
			TagName:      lowerFirst(fieldBase) + "ProviderPrefix",
			Description:  fmt.Sprintf("Provider prefix URL for %s", suffix),
			DefaultValue: prefix,
		}
	}

	dynamicFields := make([]dynamicPrefixField, 0, len(dynamicByField))
	for _, field := range dynamicByField {
		dynamicFields = append(dynamicFields, field)
	}
	sort.Slice(dynamicFields, func(i, j int) bool {
		return dynamicFields[i].FieldName < dynamicFields[j].FieldName
	})

	writeTemplate(filepath.Join(cwd, "config.gen.go"), configTemplateData{
		Package:       "config",
		DynamicFields: dynamicFields,
	}, configTemplate)
}

func resolveSpecPath(specRoot, apiPackage string) (string, error) {
	name := strings.ReplaceAll(apiPackage, "/", ".")
	candidates := []string{
		filepath.Join(specRoot, name+".yaml"),
		filepath.Join(specRoot, name+".yml"),
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("spec file not found for apiPackage %q", apiPackage)
}

func readSpec(path string) (openAPISpec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return openAPISpec{}, err
	}
	var spec openAPISpec
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		return openAPISpec{}, err
	}
	return spec, nil
}

func providerPrefixFromURL(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", err
	}
	prefix := strings.TrimSpace(parsed.Path)
	if prefix == "" {
		return "", fmt.Errorf("server url %q has no path", rawURL)
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if len(prefix) > 1 {
		prefix = strings.TrimSuffix(prefix, "/")
	}
	return prefix, nil
}

func titleFromAPIPackage(apiPackage string) string {
	parts := strings.Split(apiPackage, "/")
	if len(parts) > 1 {
		return parts[1]
	}
	return apiPackage
}

func toGoIdentifier(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return (r < '0' || r > '9') && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z')
	})
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		lower := strings.ToLower(part)
		b.WriteString(strings.ToUpper(lower[:1]))
		if len(lower) > 1 {
			b.WriteString(lower[1:])
		}
	}
	return b.String()
}

func lowerFirst(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = []rune(strings.ToLower(string(runes[0])))[0]
	return string(runes)
}

func writeTemplate(outPath string, data configTemplateData, tmpl *template.Template) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		fmt.Printf("error executing template: %v\n", err)
		return
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		fmt.Printf("error formatting generated output: %v\n", err)
		formatted = buf.Bytes()
	}

	if err := os.WriteFile(outPath, formatted, 0o644); err != nil {
		fmt.Printf("error writing config file %s: %v\n", outPath, err)
	}

}
