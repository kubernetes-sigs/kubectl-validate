package cmd_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/kubectl-validate/pkg/cmd"
)

// Shows that each testcase has its expected output when run by itself
func TestValidationErrorsIndividually(t *testing.T) {
	testcasesDir := "../../testcases"
	crdsDir := filepath.Join(testcasesDir, "crds")
	patchesDir := "../../patches/1.23"

	cases, err := os.ReadDir(testcasesDir)
	require.NoError(t, err)

	for _, f := range cases {
		ext := filepath.Ext(f.Name())
		basename := strings.TrimSuffix(f.Name(), ext)
		path := filepath.Join(testcasesDir, f.Name())
		if f.IsDir() {
			continue
		} else if ext != ".yaml" {
			continue
		}

		t.Run(basename, func(t *testing.T) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)

			lines := strings.Split(string(data), "\n")

			var comment strings.Builder
			for _, line := range lines {
				if comment.Len() == 0 && strings.TrimSpace(line) == "" {
					continue
				} else if !strings.HasPrefix(line, "#") {
					break
				} else {
					if comment.Len() != 0 {
						comment.WriteString("\n")
					}
					comment.WriteString(line[1:])
				}
			}

			expectation := metav1.Status{}
			if err := json.Unmarshal([]byte(comment.String()), &expectation); err != nil {
				t.Fatal(err)
			}

			rootCmd := cmd.NewRootCommand()

			var buf bytes.Buffer
			rootCmd.SetOut(&buf)
			rootCmd.SetArgs([]string{path})
			require.NoError(t, rootCmd.Flags().Set("version", "1.27"))
			require.NoError(t, rootCmd.Flags().Set("local-schemas", crdsDir))
			require.NoError(t, rootCmd.Flags().Set("schema-patches", patchesDir))
			require.NoError(t, rootCmd.Flags().Set("output", "json"))

			// There should be no error executing the case, just validation errors
			require.NoError(t, rootCmd.Execute())

			output := map[string]metav1.Status{}
			if err := json.Unmarshal(buf.Bytes(), &output); err != nil {
				t.Fatal(err)
			}

			require.Equal(t, expectation, output[path])
		})
	}
}
