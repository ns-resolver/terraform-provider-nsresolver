package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/ns-resolver/terraform-provider-nsresolver/internal/provider"
)

// Generate the registry docs (docs/) from the schema and examples/.
//go:generate go tool tfplugindocs generate --provider-name nsresolver

// version is set by GoReleaser at release time.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/ns-resolver/nsresolver",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err.Error())
	}
}
