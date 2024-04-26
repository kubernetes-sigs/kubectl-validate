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
		name        string
		fileSystems []fs.FS
		want        openapi.Client
	}{{
		name: "no fs",
		want: &localCRDsClient{},
	}, {
		name:        "one fs",
		fileSystems: []fs.FS{os.DirFS("test")},
		want: &localCRDsClient{
			fileSystems: []fs.FS{os.DirFS("test")},
		},
	}, {
		name:        "multiple dirs",
		fileSystems: []fs.FS{os.DirFS("test"), os.DirFS("test2")},
		want: &localCRDsClient{
			fileSystems: []fs.FS{os.DirFS("test"), os.DirFS("test2")},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewLocalCRDFiles(tt.fileSystems...)
			require.Equal(t, tt.want, got, "NewLocalCRDFiles not equal")
		})
	}
}

func Test_localCRDsClient_Paths(t *testing.T) {
	tests := []struct {
		name        string
		fileSystems []fs.FS
		want        map[string]sets.Set[string]
		wantErr     bool
	}{{
		name: "no fs",
	}, {
		name:        "one fs",
		fileSystems: []fs.FS{os.DirFS("../../testcases/crds")},
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
		name:        "two fs",
		fileSystems: []fs.FS{os.DirFS("../../testcases/crds"), os.DirFS("../../testcases/more-crds")},
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
			"apis/externaldns.k8s.io/v1alpha1": sets.New(
				"externaldns.k8s.io/v1alpha1.DNSEndpoint",
			),
		},
	}, {
		name:        "does not exist",
		fileSystems: []fs.FS{os.DirFS("../../invalid")},
		want:        map[string]sets.Set[string]{},
	}, {
		name:        "not a directory",
		fileSystems: []fs.FS{os.DirFS("../../testcases/schemas/error_not_a_dir")},
		wantErr:     true,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := NewLocalCRDFiles(tt.fileSystems...)
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
