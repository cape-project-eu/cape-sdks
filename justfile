@_default:
    just -l

# Update the submodules in the repository
update_modules:
    @echo "Update submodules"
    git submodule update --init --remote --recursive

# Build the specification in the subrepo
build_secapi_spec:
    @echo "Build secapi specification"
    cd ext/secapi && make resource-apis

# Generate necessary files inside the pulumi provider
build_pulumi_provider:
    rm -rf provider/pulumi/sdk
    rm -rf provider/pulumi/bin
    cd provider/pulumi && go generate ./...

# Build the pulumi SDK out of the provider files
build_pulumi_sdk local="true" version="0.0.0":
    rm -rf provider/pulumi/sdk
    rm -rf provider/pulumi/bin
    cd provider/pulumi && go build -o bin/pulumi-resource-cape .
    cd provider/pulumi && pulumi package gen-sdk --version {{version}} {{ if local != "false" { "--local" } else {""} }} ./

# (Re)install the pulumi SDK locally
install_pulumi_sdk:
    pulumi plugin rm resource cape -y
    pulumi plugin install resource cape 0.0.0 --file provider/pulumi/bin/pulumi-resource-cape

# Build everything pulumi related in one go
build_pulumi: build_secapi_spec build_pulumi_provider build_pulumi_sdk

# Generate mockserver files
build_mockserver:
    cd mockserver && go generate ./...

# Run the mockserver locally
run_mockserver:
    cd mockserver && go run main.go

# Build the mockserver as docker image
build_mockserver_docker tag="pulumi-cape-mockserver":
    docker build -t {{tag}} -f mockserver/Dockerfile .

# Run the previous built docker mockserver
run_mockserver_docker tag="pulumi-cape-mockserver":
    docker run --rm -p 8080:8080 -it {{tag}}
