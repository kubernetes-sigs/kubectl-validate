package openapiclient_test

import (
	"testing"

	"sigs.k8s.io/kubectl-validate/pkg/openapiclient"
)

func TestGitHubBuiltins(t *testing.T) {
	t.Parallel()
	c := openapiclient.NewGitHubBuiltins("1.27")
	_, err := c.Paths()
	if err != nil {
		t.Fatal(err)
	}
}
