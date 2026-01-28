package pulumi

//g.  :generate go run ../../internal/cmd/generator/pulumi/main.go

// package main

// import (
// 	"context"
// 	"fmt"
// 	"os"
// 	"strings"

// 	"github.com/pulumi/pulumi-go-provider/infer"
// )

// func main() {
// 	b := infer.NewProviderBuilder().
// 		WithNamespace("pulumi-cape").
// 		WithResources(
// 			infer.Resource(HelloWorld{}),
// 		)

// 	p, err := b.Build()

// 	if err != nil {
// 		panic(fmt.Errorf("unable to build provider: %w", err))
// 	}

// 	err = p.Run(context.Background(), "file", "0.1.0")

// 	if err != nil {
// 		fmt.Fprintf(os.Stderr, "Error: %s", err.Error())
// 		os.Exit(1)
// 	}
// }

// // Each resource has a controlling struct.
// type HelloWorld struct{}

// // Each resource has in input struct, defining what arguments it accepts.
// type HelloWorldArgs struct {
// 	// Fields projected into Pulumi must be public and have a `pulumi:"..."` tag.
// 	// The pulumi tag doesn't need to match the field name, but its generally a
// 	// good idea.
// 	Name string `pulumi:"name"`
// 	// Fields marked `optional` are optional, so they should have a pointer
// 	// ahead of their type.
// 	Loud *bool `pulumi:"loud,optional"`
// }

// // Each resource has a state, describing the fields that exist on the created resource.
// type HelloWorldState struct {
// 	// It is generally a good idea to embed args in outputs, but it isn't strictly necessary.
// 	HelloWorldArgs
// 	// Here we define a required output called message.
// 	Message string `pulumi:"message"`
// }

// // All resources must implement Create at a minimum.
// func (HelloWorld) Create(
// 	ctx context.Context, req infer.CreateRequest[HelloWorldArgs],
// ) (infer.CreateResponse[HelloWorldState], error) {
// 	state := HelloWorldState{HelloWorldArgs: req.Inputs}
// 	state.Message = fmt.Sprintf("Hello, %s", req.Inputs.Name)
// 	if req.Inputs.Loud != nil && *req.Inputs.Loud {
// 		state.Message = strings.ToUpper(state.Message)
// 	}
// 	return infer.CreateResponse[HelloWorldState]{
// 		Output: state,
// 	}, nil
// }
