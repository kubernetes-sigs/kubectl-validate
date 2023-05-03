package openapiclient_test

import (
	"fmt"
	"testing"

	"sigs.k8s.io/kubectl-validate/pkg/openapiclient"
)

func TestGitHubBuiltins(t *testing.T) {
	c := openapiclient.NewGitHubBuiltins("1.27")
	m, err := c.Paths()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(m)

}
