package cmd_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/kubectl-validate/pkg/cmd"
	"sigs.k8s.io/kubectl-validate/pkg/utils"
)

var (
	testcasesDir string = "../../testcases"
	manifestDir         = filepath.Join(testcasesDir, "manifests")
	crdsDir             = filepath.Join(testcasesDir, "crds")
)

// Shows that each testcase has its expected output when run by itself
func TestValidationErrorsIndividually(t *testing.T) {
	// TODO: using 1.23 since as of writing we only have patches for that schema
	// version should change to more recent version/test a matrix a versions in
	// the future.
	//!TODO: Change download-builtin-schemas to apply these patches to all
	//		 versions
	patchesDir := "../openapiclient/patches/1.23"

	cases, err := os.ReadDir(manifestDir)
	require.NoError(t, err)

	for _, f := range cases {
		path := filepath.Join(manifestDir, f.Name())
		if f.IsDir() {
			continue
		} else if !utils.IsYaml(path) {
			continue
		}

		ext := filepath.Ext(f.Name())
		basename := strings.TrimSuffix(f.Name(), ext)
		t.Run(basename, func(t *testing.T) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)

			documents, err := utils.SplitYamlDocuments(data)
			require.NoError(t, err)

			var expected []metav1.Status
			expectedError := false
			for _, document := range documents {
				if utils.IsEmptyYamlDocument(document) {
					expected = append(expected, metav1.Status{Status: metav1.StatusSuccess})
				} else {
					lines := strings.Split(string(document), "\n")

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
						t.Fatalf("error parsing leading expectation comment: %v", err)
					}

					expected = append(expected, expectation)
					if expectation.Status != "Success" {
						expectedError = true
					}
				}
			}

			rootCmd := cmd.NewRootCommand()

			var buf bytes.Buffer
			rootCmd.SetOut(&buf)
			rootCmd.SetArgs([]string{path})

			require.NoError(t, rootCmd.Flags().Set("local-crds", crdsDir))
			require.NoError(t, rootCmd.Flags().Set("schema-patches", patchesDir))
			require.NoError(t, rootCmd.Flags().Set("output", "json"))

			// There should be no error executing the case, just validation errors
			if err := rootCmd.Execute(); expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			output := map[string][]metav1.Status{}
			if err := json.Unmarshal(buf.Bytes(), &output); err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, expected, output[path])
		})
	}
}

// Test that the command returns an error if validation fails, and not when it
// succeeds
func TestReturnsError(t *testing.T) {
	path := filepath.Join(manifestDir, "error_invalid_name.yaml")
	successPath := filepath.Join(manifestDir, "configmap.yaml")

	rootCmd := cmd.NewRootCommand()
	rootCmd.SetArgs([]string{path})
	require.Error(t, rootCmd.Execute(), "expected error")

	rootCmd = cmd.NewRootCommand()
	rootCmd.SetArgs([]string{successPath})
	require.NoError(t, rootCmd.Execute(), "expected no error")
}

func TestValidateFromStdin(t *testing.T) {
	tests := []struct {
		name          string
		manifest      string
		wantDocuments int
		wantError     bool
	}{
		{
			name: "yaml",
			manifest: `apiVersion: v1
kind: ConfigMap
metadata:
  name: from-stdin
`,
			wantDocuments: 1,
		},
		{
			name:          "json",
			manifest:      `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"from-stdin"}}`,
			wantDocuments: 1,
		},
		{
			name: "multiple yaml documents",
			manifest: `apiVersion: v1
kind: ConfigMap
metadata:
  name: first
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: second
`,
			wantDocuments: 2,
		},
		{
			name: "invalid manifest",
			manifest: `apiVersion: v1
kind: ConfigMap
metadata:
  name: INVALID_NAME
`,
			wantDocuments: 1,
			wantError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootCmd := cmd.NewRootCommand()
			rootCmd.SetIn(strings.NewReader(tt.manifest))
			rootCmd.SetArgs([]string{"-", "--output=json"})

			var output bytes.Buffer
			rootCmd.SetOut(&output)

			err := rootCmd.Execute()
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			results := map[string][]metav1.Status{}
			require.NoError(t, json.Unmarshal(output.Bytes(), &results))
			require.Len(t, results["-"], tt.wantDocuments)
			for _, result := range results["-"] {
				if tt.wantError {
					assert.Equal(t, metav1.StatusFailure, result.Status)
				} else {
					assert.Equal(t, metav1.StatusSuccess, result.Status)
				}
			}
		})
	}
}

func TestValidateFileAndStdin(t *testing.T) {
	path := filepath.Join(manifestDir, "configmap.yaml")
	rootCmd := cmd.NewRootCommand()
	rootCmd.SetIn(strings.NewReader(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"from-stdin"}}`))
	rootCmd.SetArgs([]string{path, "-", "--output=json"})

	var output bytes.Buffer
	rootCmd.SetOut(&output)
	require.NoError(t, rootCmd.Execute())

	results := map[string][]metav1.Status{}
	require.NoError(t, json.Unmarshal(output.Bytes(), &results))
	require.Contains(t, results, path)
	require.Contains(t, results, "-")
}
