package openapiclient

import (
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/openapi"
	"sigs.k8s.io/kubectl-validate/pkg/openapiclient/groupversion"
)

func TestNewLocalCRDFiles(t *testing.T) {
	tests := []struct {
		name    string
		fs      fs.FS
		dirPath string
		want    openapi.Client
	}{{
		name: "fs nil and dir empty",
		want: &localCRDsClient{},
	}, {
		name:    "only dir",
		dirPath: "test",
		want: &localCRDsClient{
			dir: "test",
		},
	}, {
		name: "only fs",
		fs:   os.DirFS("."),
		want: &localCRDsClient{
			fs: os.DirFS("."),
		},
	}, {
		name:    "both fs and dir",
		fs:      os.DirFS("."),
		dirPath: "test",
		want: &localCRDsClient{
			fs:  os.DirFS("."),
			dir: "test",
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewLocalCRDFiles(tt.fs, tt.dirPath)
			require.Equal(t, tt.want, got, "NewLocalCRDFiles not equal")
		})
	}
}

func Test_localCRDsClient_Paths(t *testing.T) {
	tests := []struct {
		name    string
		fs      fs.FS
		dir     string
		want    map[string]sets.Set[string]
		wantErr bool
	}{{
		name: "fs nil and dir empty",
	}, {
		name: "only dir",
		dir:  "../../testcases/crds",
		want: map[string]sets.Set[string]{
			"apis/batch.x-k8s.io/v1alpha1": sets.New(
				"batch.x-k8s.io/v1alpha1.JobSet",
			),
			"apis/stable.example.com/v1": sets.New(
				"stable.example.com/v1.CELBasic",
			),
			"apis/acme.cert-manager.io/v1": sets.New(
				"acme.cert-manager.io/v1.Challenge",
				"acme.cert-manager.io/v1.Order",
			),
			"apis/cert-manager.io/v1": sets.New(
				"cert-manager.io/v1.Certificate",
				"cert-manager.io/v1.CertificateRequest",
				"cert-manager.io/v1.ClusterIssuer",
				"cert-manager.io/v1.Issuer",
			),
		},
	}, {
		name: "only fs",
		fs:   os.DirFS("../../testcases/crds"),
	}, {
		name: "both fs and dir",
		fs:   os.DirFS("../../testcases"),
		dir:  "crds",
		want: map[string]sets.Set[string]{
			"apis/batch.x-k8s.io/v1alpha1": sets.New(
				"batch.x-k8s.io/v1alpha1.JobSet",
			),
			"apis/stable.example.com/v1": sets.New(
				"stable.example.com/v1.CELBasic",
			),
			"apis/acme.cert-manager.io/v1": sets.New(
				"acme.cert-manager.io/v1.Challenge",
				"acme.cert-manager.io/v1.Order",
			),
			"apis/cert-manager.io/v1": sets.New(
				"cert-manager.io/v1.Certificate",
				"cert-manager.io/v1.CertificateRequest",
				"cert-manager.io/v1.ClusterIssuer",
				"cert-manager.io/v1.Issuer",
			),
		},
	}, {
		name:    "invalid dir",
		dir:     "invalid",
		wantErr: true,
	}, {
		name:    "invalid fs",
		fs:      os.DirFS("../../invalid"),
		dir:     ".",
		wantErr: true,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := NewLocalCRDFiles(tt.fs, tt.dir)
			paths, err := k.Paths()
			if (err != nil) != tt.wantErr {
				t.Errorf("localCRDsClient.Paths() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			var got map[string]sets.Set[string]
			if paths != nil {
				got = map[string]sets.Set[string]{}
				for key, value := range paths {
					got[key] = sets.New[string]()
					for component := range value.(*groupversion.OpenApiGroupVersion).Components.Schemas {
						// ignore injected schema values for test
						if !strings.HasPrefix(component, "io.k8s.apimachinery.pkg.apis.meta.v1") {
							got[key] = got[key].Insert(component)
						}
					}
				}
			}
			require.Equal(t, tt.want, got, "localCRDsClient.Paths() not equal")
		})
	}
}
