package main

import (
	"fmt"
	"os"

	"sigs.k8s.io/kubectl-validate/pkg/cmd"
)

func main() {
	rootCmd := cmd.NewRootCommand()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	}
}
