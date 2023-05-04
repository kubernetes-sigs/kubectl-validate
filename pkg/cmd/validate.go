package cmd

import (
	"context"
	"encoding/json"

	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"k8s.io/apiextensions-apiserver/pkg/apiserver"
	apiextensionsschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/registry/customresource"
	"k8s.io/apiextensions-apiserver/pkg/registry/customresourcedefinition"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kubectl-validate/pkg/openapiclient"
	"sigs.k8s.io/kubectl-validate/pkg/validatorfactory"
	"sigs.k8s.io/yaml"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
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
	localFilesDir       string
	schemaPatchesDir    string
	outputFormat        OutputFormat
}

func NewRootCommand() *cobra.Command {
	invoked := &commandFlags{
		outputFormat: OutputHuman,
		version:      "1.27",
	}
	res := &cobra.Command{
		Use:   "kubectl-validate [manifests to validate]",
		Short: "kubectl-validate",
		Long:  "kubectl-validate is a CLI tool to validate Kubernetes manifests against their schemas",
		Args:  cobra.MinimumNArgs(1),
		RunE:  invoked.Run,
	}
	res.Flags().StringVarP(&invoked.version, "version", "", "", "Kubernetes version to validate native resources against. Required if not connected directly to cluster")
	res.Flags().StringVarP(&invoked.localFilesDir, "local-schemas", "l", "", "--local-schemas=./path/to/schemas/dir. Path to a directory with format: /apis/<group>/<version>.json for each group-version's schema.")
	res.Flags().StringVarP(&invoked.schemaPatchesDir, "schema-patches", "", "", "Path to a directory with format: /apis/<group>/<version>.json for each group-version's schema you wish to jsonpatch to the groupversion's final schema. Patches only apply if the schema exists")
	res.Flags().VarP(&invoked.outputFormat, "output", "o", "Output format. Choice of: \"human\" or \"json\"")
	clientcmd.BindOverrideFlags(&invoked.kubeConfigOverrides, res.Flags(), clientcmd.RecommendedConfigOverrideFlags("kube-"))
	return res
}

func (c *commandFlags) Run(cmd *cobra.Command, args []string) error {
	factory, err := validatorfactory.New(
		openapiclient.NewOverlayClient(
			openapiclient.PatchLoaderFromDirectory(filepath.Base(c.schemaPatchesDir), os.DirFS(filepath.Dir(c.schemaPatchesDir))),
			openapiclient.NewComposite(
				// Tool fetches openapi in the following priority order:
				openapiclient.NewLocalFiles(c.localFilesDir), // satisfy expectation users' expectation that provided local files are used
				openapiclient.NewLocalCRDFiles(c.localFilesDir),
				openapiclient.NewKubeConfig(c.kubeConfigOverrides), // contact connected cluster for any schemas. (should this be opt-in?)
				openapiclient.NewHardcodedBuiltins(c.version),      // schemas for known k8s versions are scraped from GH and placed here
				openapiclient.NewGitHubBuiltins(c.version),         // check github for builtins not hardcoded. subject to rate limiting. should use a diskcache since etag requests are not limited
			)))
	if err != nil {
		return err
	}

	var files []string
	for _, fileOrDir := range args {
		if info, err := os.Stat(fileOrDir); err != nil {
			return err
		} else if info.IsDir() {
			dirFiles, err := os.ReadDir(fileOrDir)
			if err != nil {
				return err
			}

			for _, v := range dirFiles {
				path := filepath.Join(fileOrDir, v.Name())
				ext := strings.ToLower(filepath.Ext(path))
				if ext == ".json" || ext == ".yaml" || ext == ".yml" {
					files = append(files, path)
				} else {
					if c.outputFormat == OutputHuman {
						fmt.Printf("skipping %v since it is not json or yaml\n", path)
					}
				}
			}
		} else {
			files = append(files, fileOrDir)
		}

	}

	if c.outputFormat == OutputHuman {
		for _, path := range files {
			fmt.Fprintf(cmd.OutOrStdout(), "\n\033[1m%v\033[0m...", path)
			e := ValidateFile(path, factory)
			if e != nil {
				fmt.Fprintln(cmd.OutOrStdout(), "\033[31mERROR\033[0m")
				fmt.Fprintln(cmd.ErrOrStderr(), e.Error())
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "\033[32mOK\033[0m")
			}
		}
	} else {
		res := map[string]metav1.Status{}
		for _, path := range files {
			valErr := ValidateFile(path, factory)
			var statusErr *k8serrors.StatusError
			var fieldError *field.Error
			var errorList utilerrors.Aggregate

			if errors.As(valErr, &statusErr) {
				res[path] = statusErr.ErrStatus
			} else if errors.As(valErr, &fieldError) {
				res[path] = k8serrors.NewInvalid(schema.GroupKind{}, "", field.ErrorList{fieldError}).ErrStatus
			} else if errors.As(valErr, &errorList) {
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
					res[path] = k8serrors.NewInvalid(schema.GroupKind{}, "", fieldErrs).ErrStatus
				} else {
					res[path] = k8serrors.NewInternalError(errors.Join(otherErrs...)).ErrStatus
				}

			} else if valErr != nil {
				res[path] = k8serrors.NewInternalError(valErr).ErrStatus
			} else {
				res[path] = metav1.Status{Status: metav1.StatusSuccess}
			}
		}
		data, e := json.MarshalIndent(res, "", "    ")
		if e != nil {
			return fmt.Errorf("failed to render results into JSON: %w", e)
		}

		fmt.Fprintln(cmd.OutOrStdout(), string(data))
	}

	return nil
}

func ValidateFile(filePath string, resolver *validatorfactory.ValidatorFactory) error {
	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	metadata := metav1.TypeMeta{}
	if err = yaml.Unmarshal(fileBytes, &metadata); err != nil {
		return fmt.Errorf("failed to parse yaml: %w", err)
	}

	gvk := metadata.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		return fmt.Errorf("GVK cannot be empty")
	}

	// CRD spec contains an infinite loop which is not supported by K8s
	// OpenAPI-based validator. Use the handwritten validation based upon
	// native types for CRD files. There are no other recursive schemas to my
	// knowledge, and any schema defined in CRD cannot be recursive.
	if gvk.Group == "apiextensions.k8s.io" && gvk.Kind == "CustomResourceDefinition" {
		obj, _, err := serializer.NewCodecFactory(apiserver.Scheme).UniversalDecoder().Decode(fileBytes, nil, nil)
		if err != nil {
			return err
		}

		strat := customresourcedefinition.NewStrategy(apiserver.Scheme)
		rest.FillObjectMetaSystemFields(obj.(metav1.Object))
		return rest.BeforeCreate(strat, request.WithNamespace(context.TODO(), ""), obj)
	}

	validators, err := resolver.ValidatorsForGVK(gvk)
	if err != nil {
		return fmt.Errorf("failed to retrieve validator: %w", err)
	}

	// Grab structural schema for use in several of the validation functions.
	// The validators use a weird mix of structural schema and openapi
	ss, err := validators.StructuralSchema()
	if err != nil || ss == nil {
		return err
	}

	// Fetch a decoder to decode this object from its structural schema
	decoder, err := validators.Decoder(gvk)
	if err != nil {
		return err
	}

	const mediaType = runtime.ContentTypeYAML
	info, ok := runtime.SerializerInfoForMediaType(decoder.SupportedMediaTypes(), mediaType)
	if !ok {
		return fmt.Errorf("unsupported media type %q", mediaType)
	}

	dec := decoder.DecoderToVersion(info.StrictSerializer, gvk.GroupVersion())
	runtimeObj, _, err := dec.Decode(fileBytes, &gvk, &unstructured.Unstructured{})
	if err != nil {
		return err
	}

	obj := runtimeObj.(*unstructured.Unstructured)

	_, err = meta.Accessor(obj)
	if err != nil {
		return field.Invalid(field.NewPath("metadata"), nil, err.Error())
	}

	//!TODO: source this information from OpenAPI somehow
	crdIsNamespaceScoped := true

	// Infer namespace scoped based on presence of namespace field in user data
	// for now :(
	if n := obj.GetNamespace(); len(n) == 0 {
		crdIsNamespaceScoped = false
	}
	if obj.GetAPIVersion() == "v1" {
		// CRD validator expects unconditoinal slashes and nonempty group,
		// since it is not originally intended for built-in
		gvk.Group = "core"
		obj.SetAPIVersion("core/v1")
	}

	strat := customresource.NewStrategy(validators.ObjectTyper(gvk), crdIsNamespaceScoped, gvk, validators.SchemaValidator(), nil, map[string]*apiextensionsschema.Structural{
		gvk.Version: ss,
	}, nil, nil)

	rest.FillObjectMetaSystemFields(obj)
	return rest.BeforeCreate(strat, request.WithNamespace(context.TODO(), obj.GetNamespace()), obj)

}
