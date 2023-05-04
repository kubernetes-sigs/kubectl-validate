package main_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kubectl-validate/pkg/cmd"
	"sigs.k8s.io/kubectl-validate/pkg/openapiclient"
	"sigs.k8s.io/kubectl-validate/pkg/validatorfactory"
)

func TestMain(t *testing.T) {
	factory, err := validatorfactory.New(openapiclient.NewKubeConfig(clientcmd.ConfigOverrides{}))
	if err != nil {
		panic(err)
	}

	dir := "../../testcases"
	files, err := os.ReadDir(dir)
	if err != nil {
		panic(err)
	}
	for _, entry := range files {
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			if filepath.Ext(name) == ".yaml" || filepath.Ext(name) == "json" {
				path := filepath.Join(dir, name)
				e := cmd.ValidateFile(path, factory)
				if e != nil {
					if strings.HasPrefix(name, "error_") {
						t.Log(name, e.Error())
					} else {
						t.Error(name, e.Error())
					}
				} else {
					if strings.HasPrefix(name, "error_") {
						t.Error(name, "expected error")
					} else {
						t.Log(name, "success")
					}
				}
			}
		})
	}
}
