package main

import (
	"os"

	"github.com/relaygate/relaygate/core/cli"
)

var version = "dev"

func main() {
	cli.Version = version
	os.Exit(cli.Run(os.Args[1:]))
}
