package main

import (
	"os"

	"github.com/sokoloff/aws-datalake-tools/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
