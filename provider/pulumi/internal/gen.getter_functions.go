//go:build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cape-project.eu/provider/pulumi/internal/codegen"
)

const SchemasDir = "../../../ext/secapi/spec/schemas"
const PulumiControlResourceFile = "../pulumi.gen.yaml"
const SchemasImportPath = "cape-project.eu/provider/pulumi/internal/schemas"

var getterFunTmpl = codegen.ReadTemplate("getter_functions", "codegen/getter_functions.tmpl")

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
	_ = codegen.NewSchemaResolver(models)

	genYaml, err := codegen.GetPulumiGenYaml(controlPath)
	if err != nil {
		panic(err)
	}

	for packageName, functions := range genYaml.GetterFunctions {
		for functionName, function := range functions {
			writeTemplate(fmt.Sprintf("./%s/%s.gen.go", packageName, strings.ToLower(functionName)), tmplData{
				Package:                 packageName,
				Name:                    functionName,
				APIPackage:              function.APIPackage,
				WithoutWorkspace:        function.WithoutWorkspace,
				WithoutTenant:           function.WithoutTenant,
				ClientFunction:          function.ClientFunction,
				OutputType:              function.OutputType,
				ResponseType:            function.ResponseType,
				ProviderPrefixOverwrite: function.ProviderPrefixOverwrite,
			})
		}
	}
}

type tmplData struct {
	Package                 string
	Name                    string
	APIPackage              string
	WithoutWorkspace        bool
	WithoutTenant           bool
	ClientFunction          string
	OutputType              string
	ResponseType            string
	ProviderPrefixOverwrite *string
}

func writeTemplate(outPath string, data tmplData) {
	dir := filepath.Dir(outPath)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		fmt.Println("Error creating directory:", err)
		return
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		fmt.Println("error creating/opening file:", err)
		return
	}
	defer func() {
		_ = outFile.Close()
	}()
	if err := getterFunTmpl.Execute(outFile, data); err != nil {
		fmt.Println("error executing template:", err)
	}
}
