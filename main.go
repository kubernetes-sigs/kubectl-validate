package main

import (
	"os"

	"sigs.k8s.io/kubectl-validate/pkg/cmd"
)

func main() {
	rootCmd := cmd.NewRootCommand()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
