package main

import (
	"os"

	"sigs.k8s.io/kubectl-validate/pkg/cmd"
)

func main() {
	rootCmd := cmd.NewRootCommand()
	if err := rootCmd.Execute(); err != nil {
		switch err.(type) {
		case cmd.ValidationError:
			os.Exit(1)
		case cmd.ArgumentError:
			os.Exit(2)
		case cmd.InternalError:
			os.Exit(3)
		default:
			// This case should not get hit, but in case it does,
			// Treat unknown error as internal error
			os.Exit(3)
		}
	}
}
