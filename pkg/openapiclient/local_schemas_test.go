package openapiclient

import (
	"io/fs"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/openapi"
)

func TestNewLocalSchemaFiles(t *testing.T) {
	tests := []struct {
		name string
		fs   fs.FS
		want openapi.Client
	}{{
		name: "without fs",
		want: &localSchemasClient{},
	}, {
		name: "with fs",
		fs:   os.DirFS("."),
		want: &localSchemasClient{
			fs: os.DirFS("."),
		},
	}, {
		name: "with sub fs",
		fs: func() fs.FS {
			sub, err := fs.Sub(os.DirFS("."), "test")
			assert.NoError(t, err)
			return sub
		}(),
		want: &localSchemasClient{
			fs: func() fs.FS {
				sub, err := fs.Sub(os.DirFS("."), "test")
				assert.NoError(t, err)
				return sub
			}(),
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewLocalSchemaFiles(tt.fs)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_localSchemasClient_Paths(t *testing.T) {
	tests := []struct {
		name    string
		fs      fs.FS
		want    sets.Set[string]
		wantErr bool
	}{{
		name: "without fs",
	}, {
		name: "with fs",
		fs:   os.DirFS("./builtins/1.27"),
		want: sets.New(
			"api/v1",
			"apis/admissionregistration.k8s.io/v1",
			"apis/admissionregistration.k8s.io/v1alpha1",
			"apis/apiextensions.k8s.io/v1",
			"apis/apiregistration.k8s.io/v1",
			"apis/apps/v1",
			"apis/authentication.k8s.io/v1",
			"apis/authentication.k8s.io/v1alpha1",
			"apis/authentication.k8s.io/v1beta1",
			"apis/authorization.k8s.io/v1",
			"apis/autoscaling/v1",
			"apis/autoscaling/v2",
			"apis/batch/v1",
			"apis/certificates.k8s.io/v1",
			"apis/certificates.k8s.io/v1alpha1",
			"apis/coordination.k8s.io/v1",
			"apis/discovery.k8s.io/v1",
			"apis/events.k8s.io/v1",
			"apis/flowcontrol.apiserver.k8s.io/v1beta2",
			"apis/flowcontrol.apiserver.k8s.io/v1beta3",
			"apis/internal.apiserver.k8s.io/v1alpha1",
			"apis/networking.k8s.io/v1",
			"apis/networking.k8s.io/v1alpha1",
			"apis/node.k8s.io/v1",
			"apis/policy/v1",
			"apis/rbac.authorization.k8s.io/v1",
			"apis/resource.k8s.io/v1alpha2",
			"apis/scheduling.k8s.io/v1",
			"apis/storage.k8s.io/v1",
		),
	}, {
		name: "invalid fs",
		fs:   os.DirFS("../../invalid"),
		want: sets.New[string](),
	}, {
		name:    "not a dir",
		fs:      os.DirFS("../../testcases/schemas/error_not_a_dir"),
		wantErr: true,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := NewLocalSchemaFiles(tt.fs)
			paths, err := k.Paths()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			var got sets.Set[string]
			if paths != nil {
				got = sets.New[string]()
				for key := range paths {
					got.Insert(key)
				}
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
