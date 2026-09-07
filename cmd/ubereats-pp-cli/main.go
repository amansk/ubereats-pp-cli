package main

import (
	"os"

	"github.com/amansk/ubereats-pp-cli/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
