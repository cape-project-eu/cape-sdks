# CAPE SDK Generator

Generator workspace for CAPE resources derived from SecAPI.

## Repository layout

- `examples/`: provider usage examples (currently Pulumi; Nitric planned later).
- `ext/`: git submodules, currently the SecAPI reference (`ext/secapi`).
- `mockserver/`: in-memory mock application that behaves like SecAPI.
- `provider/pulumi/`: Pulumi provider implementation for CAPE resources from SecAPI.
- `justfile`: task runner entrypoint for generation, build, install, and local runs.

## Preparations for development

Install these tools first:

- Go
- `just`
- Pulumi CLI
- Docker (needed for mockserver container workflows)

Then initialize submodules:

```bash
just update_modules
```

## Development workflows

Generate/build provider artifacts:

```bash
just build_secapi_spec
just build_pulumi_provider
just build_pulumi_sdk
# or all at once:
just build_pulumi
```

Generate/run mockserver:

```bash
just build_mockserver
just run_mockserver
```

Mockserver via Docker:

```bash
just build_mockserver_docker
just run_mockserver_docker
```

## Run examples

To run examples, utilize the mockserver.
First, build the SDK with `just build_pulumi`, then setup the examples with `just setup_examples`.
Setup examples install the required pulumi sdks inside the example directories.

Then you will need to run the mockserver (either via docker or natively).

After the mockserver has been started, cd into the example directory.

There, the pulumi workflow starts. The "dev" stack has already been prepared. So on a fresh maschine,
you will need to import the pre-existing stack after logging in locally.

1. `pulumi login --local` - this logs in locally without any S3 backend or similar.
2. `pulumi stack init dev` - this initializes the local dev stack. The passphrase is empty string.
3. `pulumi up` - will then calculate the diff (non-existing after fresh run) and runs the diff. the user
   then may accept the changes and the stack is deployed against the mock-server.
4. optional: make changes to the pulumi program and redeploy with `pulumi up`.
5. `pulumi down` - this then shuts down the stack again and deletes all stack resources.
6. shutdown the mockserver - everything inside the memory of the server is removed. you may start again.
