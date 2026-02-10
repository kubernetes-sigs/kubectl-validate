package cmd_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/kubectl-validate/pkg/cmd"
	"sigs.k8s.io/kubectl-validate/pkg/openapiclient"
	"sigs.k8s.io/kubectl-validate/pkg/utils"
	"sigs.k8s.io/kubectl-validate/pkg/validator"
)

var (
	testcasesDir string = "../../testcases"
	manifestDir         = filepath.Join(testcasesDir, "manifests")
	crdsDir             = filepath.Join(testcasesDir, "crds")
)

func newBuiltinValidator(t *testing.T) *validator.Validator {
	t.Helper()
	val, err := validator.New(openapiclient.NewHardcodedBuiltins("1.30"))
	require.NoError(t, err)
	return val
}

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

func TestValidateDocumentAllowsMixedList(t *testing.T) {
	resolver := newBuiltinValidator(t)
	doc, err := os.ReadFile(filepath.Join(manifestDir, "list_configmaps_valid.yaml"))
	require.NoError(t, err)

	require.NoError(t, cmd.ValidateDocument(doc, resolver))
}

func TestValidateDocumentRejectsNestedList(t *testing.T) {
	resolver := newBuiltinValidator(t)
	doc, err := os.ReadFile(filepath.Join(manifestDir, "list_nested_invalid.yaml"))
	require.NoError(t, err)

	err = cmd.ValidateDocument(doc, resolver)
	require.Error(t, err)

	var statusErr *k8serrors.StatusError
	require.True(t, errors.As(err, &statusErr))
	assert.Equal(t, metav1.StatusReasonInvalid, statusErr.ErrStatus.Reason)
	if assert.NotNil(t, statusErr.ErrStatus.Details) && assert.NotEmpty(t, statusErr.ErrStatus.Details.Causes) {
		assert.Contains(t, statusErr.ErrStatus.Details.Causes[0].Message, "List kinds may only appear at the document root")
	}
}
