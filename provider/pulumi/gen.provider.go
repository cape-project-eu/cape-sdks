//go:build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/template"

	"cape-project.eu/sdk-generator/provider/pulumi/internal/codegen"
)

const PulumiControlResourceFile = "pulumi.gen.yaml"
const ProviderTemplatePath = "internal/codegen/provider.tmpl"
const PulumiPluginTemplatePath = "internal/codegen/pulumi_plugin.tmpl"
const ResourceImportBase = "cape-project.eu/sdk-generator/provider/pulumi/internal"

var providerTemplate = codegen.ReadTemplate("provider", ProviderTemplatePath)
var pulumiPluginTemplate = codegen.ReadTemplate("pulumiplugin", PulumiPluginTemplatePath)

type providerImport struct {
	Alias string
	Path  string
}

type providerResource struct {
	Name         string
	Package      string
	PackageAlias string
}

type providerTemplateData struct {
	Name          string
	DisplayName   string
	Description   string
	Namespace     string
	PulumiGenYaml codegen.PulumiGenYaml
	Imports       []providerImport
	Resources     []providerResource
}

func main() {
	cwd, _ := os.Getwd()
	controlPath := PulumiControlResourceFile
	if !filepath.IsAbs(controlPath) {
		controlPath = filepath.Join(cwd, controlPath)
	}

	genYaml, err := codegen.GetPulumiGenYaml(controlPath)
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

	resourceDefs := make([]providerResource, 0, len(resourceNames))
	importsByAlias := map[string]string{}
	for _, name := range resourceNames {
		spec := resources[name]
		if spec.Package == "" {
			continue
		}
		alias := spec.Package
		importsByAlias[alias] = filepath.ToSlash(filepath.Join(ResourceImportBase, spec.Package))
		resourceDefs = append(resourceDefs, providerResource{
			Name:         name,
			Package:      spec.Package,
			PackageAlias: alias,
		})
	}

	imports := make([]providerImport, 0, len(importsByAlias))
	for alias, path := range importsByAlias {
		imports = append(imports, providerImport{
			Alias: alias,
			Path:  path,
		})
	}
	sort.Slice(imports, func(i, j int) bool {
		return imports[i].Alias < imports[j].Alias
	})

	data := providerTemplateData{
		Name:          "cape",
		DisplayName:   "pulumi-cape",
		Description:   "A pulumi provider built for CAPE Project resources.",
		Namespace:     "pulumi",
		Imports:       imports,
		Resources:     resourceDefs,
		PulumiGenYaml: genYaml,
	}

	writeTemplate(filepath.Join(cwd, "provider.gen.go"), data, providerTemplate)
	writeTemplate(filepath.Join(cwd, "PulumiPlugin.yaml"), data, pulumiPluginTemplate)
}

func writeTemplate(outPath string, data providerTemplateData, tmpl *template.Template) {
	outFile, err := os.Create(outPath)
	if err != nil {
		println(fmt.Errorf("error creating/opening file: %s", err))
		return
	}
	defer func() {
		_ = outFile.Close()
	}()
	if err := tmpl.Execute(outFile, data); err != nil {
		println(fmt.Errorf("error executing template: %s", err))
	}
}
