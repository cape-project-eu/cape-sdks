//go:build ignore

package main

import (
	"bytes"
	"fmt"
	"go/format"
	"net/url"
	"text/template"

	"os"
	"path/filepath"

	"go.yaml.in/yaml/v4"

	"cape-project.eu/provider/pulumi/internal/codegen"
)

const (
	SecapiSpecDir      = "../../../ext/secapi/spec"
	ConfigTemplatePath = "../internal/codegen/config.tmpl"
)

var configTemplate = codegen.ReadTemplate("config", ConfigTemplatePath)

type dynamicPrefixField struct {
	Name         string
	Version      string
	DefaultValue string
}

type openAPISpec struct {
	Servers []struct {
		URL         string `yaml:"url"`
		Description string `yaml:"description"`
	} `yaml:"servers"`
	Info struct {
		Title   string `yaml:"title"`
		Version string `yaml:"version"`
	} `yaml:"info"`
}

func main() {
	cwd, _ := os.Getwd()

	specRoot := SecapiSpecDir
	if !filepath.IsAbs(specRoot) {
		specRoot = filepath.Join(cwd, specRoot)
	}

	files, err := os.ReadDir(specRoot)
	if err != nil {
		fmt.Printf("error reading spec dir: %v\n", err)
		return
	}

	dynamicFields := make(map[string]dynamicPrefixField, 0)

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		spec, err := readSpec(filepath.Join(specRoot, file.Name()))
		if err != nil {
			fmt.Printf("error reading spec: %v\n", err)
			return
		}

		if _, ok := dynamicFields[spec.Info.Title]; ok {
			continue
		}

		if spec.Servers[0].Description != "Path Schema" {
			continue
		}

		uri, err := url.Parse(spec.Servers[0].URL)
		if err != nil {
			fmt.Printf("error parsing server URL: %v\n", err)
			return
		}

		dynamicFields[spec.Info.Title] = dynamicPrefixField{
			Name:         spec.Info.Title,
			Version:      spec.Info.Version,
			DefaultValue: uri.Path,
		}
	}

	writeTemplate(filepath.Join(cwd, "config.gen.go"), dynamicFields, configTemplate)
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

func writeTemplate(outPath string, data map[string]dynamicPrefixField, tmpl *template.Template) {
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
