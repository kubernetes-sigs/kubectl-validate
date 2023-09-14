package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/apiextensions-apiserver/pkg/apiserver"
	"k8s.io/apiextensions-apiserver/pkg/registry/customresourcedefinition"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kubectl-validate/pkg/openapiclient"
	"sigs.k8s.io/kubectl-validate/pkg/utils"
	"sigs.k8s.io/kubectl-validate/pkg/validatorfactory"

	yamlv2 "gopkg.in/yaml.v2"
)

type OutputFormat string

const (
	OutputHuman OutputFormat = "human"
	OutputJSON  OutputFormat = "json"
)

// String is used both by fmt.Print and by Cobra in help text
func (e *OutputFormat) String() string {
	return string(*e)
}

// Set must have pointer receiver so it doesn't change the value of a copy
func (e *OutputFormat) Set(v string) error {
	switch v {
	case "human", "json":
		*e = OutputFormat(v)
		return nil
	default:
		return fmt.Errorf(`must be one of "human", or "json"`)
	}
}

// Type is only used in help text
func (e *OutputFormat) Type() string {
	return "OutputFormat"
}

type commandFlags struct {
	kubeConfigOverrides clientcmd.ConfigOverrides
	version             string
	localSchemasDir     string
	localCRDsDir        string
	schemaPatchesDir    string
	outputFormat        OutputFormat
}

func NewRootCommand() *cobra.Command {
	invoked := &commandFlags{
		outputFormat: OutputHuman,
		version:      "1.27",
	}
	res := &cobra.Command{
		Use:          "kubectl-validate [manifests to validate]",
		Short:        "kubectl-validate",
		Long:         "kubectl-validate is a CLI tool to validate Kubernetes manifests against their schemas",
		Args:         cobra.MinimumNArgs(1),
		RunE:         invoked.Run,
		SilenceUsage: true,
	}
	res.Flags().StringVarP(&invoked.version, "version", "", invoked.version, "Kubernetes version to validate native resources against. Required if not connected directly to cluster")
	res.Flags().StringVarP(&invoked.localSchemasDir, "local-schemas", "", "", "--local-schemas=./path/to/schemas/dir. Path to a directory with format: /apis/<group>/<version>.json for each group-version's schema.")
	res.Flags().StringVarP(&invoked.localCRDsDir, "local-crds", "", "", "--local-crds=./path/to/crds/dir. Path to a directory containing .yaml or .yml files for CRD definitions.")
	res.Flags().StringVarP(&invoked.schemaPatchesDir, "schema-patches", "", "", "Path to a directory with format: /apis/<group>/<version>.json for each group-version's schema you wish to jsonpatch to the groupversion's final schema. Patches only apply if the schema exists")
	res.Flags().VarP(&invoked.outputFormat, "output", "o", "Output format. Choice of: \"human\" or \"json\"")
	clientcmd.BindOverrideFlags(&invoked.kubeConfigOverrides, res.Flags(), clientcmd.RecommendedConfigOverrideFlags("kube-"))
	return res
}

type joinedErrors interface {
	Unwrap() []error
}

func errorToStatus(err error) metav1.Status {
	var statusErr *k8serrors.StatusError
	var fieldError *field.Error
	var errorList utilerrors.Aggregate
	var errorList2 joinedErrors
	if errors.As(err, &errorList2) {
		errs := errorList2.Unwrap()
		if len(errs) == 0 {
			return metav1.Status{Status: metav1.StatusSuccess}
		}
		var fieldErrors field.ErrorList
		var otherErrors []error
		var yamlErrors []metav1.StatusCause

		for _, e := range errs {
			var fieldError *field.Error
			var yamlError *yamlv2.TypeError

			if errors.As(e, &fieldError) {
				fieldErrors = append(fieldErrors, fieldError)
			} else if errors.As(e, &yamlError) {
				for _, sub := range yamlError.Errors {
					yamlErrors = append(yamlErrors, metav1.StatusCause{
						Message: sub,
					})
				}
			} else {
				otherErrors = append(otherErrors, e)
			}
		}

		if len(otherErrors) > 0 {
			return k8serrors.NewInternalError(err).ErrStatus
		} else if len(yamlErrors) > 0 && len(fieldErrors) == 0 {
			// YAML type errors are raised during decoding
			return metav1.Status{
				Status: metav1.StatusFailure,
				Code:   http.StatusBadRequest,
				Reason: metav1.StatusReasonBadRequest,
				Details: &metav1.StatusDetails{
					Causes: yamlErrors,
				},
				Message: "failed to unmarshal document to YAML",
			}
		}
		return k8serrors.NewInvalid(schema.GroupKind{}, "", fieldErrors).ErrStatus
	} else if errors.As(err, &statusErr) {
		return statusErr.ErrStatus
	} else if errors.As(err, &fieldError) {
		return k8serrors.NewInvalid(schema.GroupKind{}, "", field.ErrorList{fieldError}).ErrStatus
	} else if errors.As(err, &errorList) {
		errs := errorList.Errors()
		var fieldErrs []*field.Error
		var otherErrs []error
		for _, e := range errs {
			fieldError = nil
			if errors.As(e, &fieldError) {
				fieldErrs = append(fieldErrs, fieldError)
			} else {
				otherErrs = append(otherErrs, e)
			}
		}
		if len(otherErrs) == 0 {
			return k8serrors.NewInvalid(schema.GroupKind{}, "", fieldErrs).ErrStatus
		} else {
			return k8serrors.NewInternalError(err).ErrStatus
		}
	} else if err != nil {
		return k8serrors.NewInternalError(err).ErrStatus
	}
	return metav1.Status{Status: metav1.StatusSuccess}
}

func (c *commandFlags) Run(cmd *cobra.Command, args []string) error {
	// tool fetches openapi in the following priority order:
	factory, err := validatorfactory.New(
		openapiclient.NewOverlay(
			// apply user defined patches on top of the final schema
			openapiclient.PatchLoaderFromDirectory(nil, c.schemaPatchesDir),
			openapiclient.NewComposite(
				// consult local OpenAPI
				openapiclient.NewLocalSchemaFiles(nil, c.localSchemasDir),
				// consult local CRDs
				openapiclient.NewLocalCRDFiles(nil, c.localCRDsDir),
				openapiclient.NewOverlay(
					// Hand-written hardcoded patches.
					openapiclient.HardcodedPatchLoader(c.version),
					// try cluster for schemas first, if they are not available
					// then fallback to hardcoded or builtin schemas
					openapiclient.NewFallback(
						// contact connected cluster for any schemas. (should this be opt-in?)
						openapiclient.NewKubeConfig(c.kubeConfigOverrides),
						// try hardcoded builtins first, if they are not available
						// fall back to GitHub builtins
						openapiclient.NewFallback(
							// schemas for known k8s versions are scraped from GH and placed here
							openapiclient.NewHardcodedBuiltins(c.version),
							// check github for builtins not hardcoded.
							// subject to rate limiting. should use a diskcache
							// since etag requests are not limited
							openapiclient.NewGitHubBuiltins(c.version),
						)),
				),
			),
		),
	)
	if err != nil {
		return ArgumentError{err}
	}

	files, err := utils.FindFiles(args...)
	if err != nil {
		return ArgumentError{err}
	}

	hasError := false
	if c.outputFormat == OutputHuman {
		for _, path := range files {
			fmt.Fprintf(cmd.OutOrStdout(), "\n\033[1m%v\033[0m...", path)
			var errs []error
			for _, err := range ValidateFile(path, factory) {
				if err != nil {
					errs = append(errs, err)
				}
			}
			if len(errs) != 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "\033[31mERROR\033[0m")
				for _, err := range errs {
					fmt.Fprintln(cmd.ErrOrStderr(), err.Error())
				}
				hasError = true
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "\033[32mOK\033[0m")
			}
		}
	} else {
		res := map[string][]metav1.Status{}
		for _, path := range files {
			for _, err := range ValidateFile(path, factory) {
				res[path] = append(res[path], errorToStatus(err))
				hasError = hasError || err != nil
			}
		}
		data, e := json.MarshalIndent(res, "", "    ")
		if e != nil {
			return InternalError{fmt.Errorf("failed to render results into JSON: %w", e)}
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
	}

	if hasError {
		return ValidationError{errors.New("validation failed")}
	}
	return nil
}

func ValidateFile(filePath string, resolver *validatorfactory.ValidatorFactory) []error {
	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return []error{fmt.Errorf("error reading file: %w", err)}
	}
	if utils.IsYaml(filePath) {
		documents, err := utils.SplitYamlDocuments(fileBytes)
		if err != nil {
			return []error{err}
		}
		var errs []error
		for _, document := range documents {
			if utils.IsEmptyYamlDocument(document) {
				errs = append(errs, nil)
			} else {
				errs = append(errs, ValidateDocument(document, resolver))
			}
		}
		return errs
	} else {
		return []error{
			ValidateDocument(fileBytes, resolver),
		}
	}
}

func ValidateDocument(document []byte, resolver *validatorfactory.ValidatorFactory) error {
	gvk, parsed, err := resolver.Parse(document)
	if gvk.Group == "apiextensions.k8s.io" && gvk.Kind == "CustomResourceDefinition" {
		// CRD spec contains an infinite loop which is not supported by K8s
		// OpenAPI-based validator. Use the handwritten validation based upon
		// native types for CRD files. There are no other recursive schemas to my
		// knowledge, and any schema defined in CRD cannot be recursive.
		// Long term goal is to remove this once k8s upstream has better
		// support for validating against spec.Schema for native types.
		obj, _, err := serializer.NewCodecFactory(apiserver.Scheme).UniversalDecoder().Decode(document, nil, nil)
		if err != nil {
			return err
		}

		strat := customresourcedefinition.NewStrategy(apiserver.Scheme)
		rest.FillObjectMetaSystemFields(obj.(metav1.Object))
		return rest.BeforeCreate(strat, request.WithNamespace(context.TODO(), ""), obj)
	} else if err != nil {
		return err
	}
	return resolver.Validate(parsed)
}
