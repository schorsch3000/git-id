package main

import (
	_ "git-id/internal/termquiet" // before bubbletea initialises, see the package comment
	"os"

	"git-id/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
