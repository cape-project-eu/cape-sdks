package main

//go:generate find . -name "*.gen.go" -not -name "gen.go" -delete
//go:generate rm -rf PulumiPlugin.yaml

//go:generate sh -c "cd config && go run gen.config.go"
//go:generate sh -c "cd secapi/models && ./gen_models.sh"
//go:generate sh -c "cd secapi && ./gen_apis.sh"
//go:generate sh -c "cd internal/schemas && go run gen.schemas.go"
//go:generate sh -c "cd internal && go run gen.controlresources.go"
//go:generate go run gen.provider.go

//go:generate go run github.com/jmattheis/goverter/cmd/goverter gen ./...
