package openapiclient_test

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/version"
	"sigs.k8s.io/kubectl-validate/pkg/openapiclient"
)

var LinkedK8sVersion *version.Version = func() *version.Version {
	cmd := exec.Command("go", "list", "-m", "-mod=mod", "-f", "{{if eq .Path \"k8s.io/api\"}}{{.Version}}{{end}}", "all")
	cmd.Dir = ""
	out, err := cmd.Output()
	if err != nil {
		panic(err)
	}

	return version.MustParseSemantic(strings.TrimSpace(string(out)))
}()

func TestHasUptoDateBuiltinSchemas(t *testing.T) {
	t.Parallel()
	t.Log(LinkedK8sVersion)

	if LinkedK8sVersion.Major() != 0 {
		t.Fatalf("Major version of linked k8s.io/api is not 0: %v", LinkedK8sVersion)
	}

	for i := 23; i <= int(LinkedK8sVersion.Minor()); i++ {
		found := false
		for _, version := range openapiclient.HardcodedBuiltinVersions {
			if version == "1."+fmt.Sprint(i) {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Missing builtin version v1.%d", i)
		}
	}
}
