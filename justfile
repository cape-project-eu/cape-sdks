

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

bundle_secapi_schematas:
    npx @redocly/cli join ext/secapi/spec/schemas/*.yaml -o foo.yaml