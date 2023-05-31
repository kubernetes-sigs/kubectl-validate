package openapiclient

import (
	"io/fs"
	"os"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/openapi"
)

func TestNewLocalSchemaFiles(t *testing.T) {
	tests := []struct {
		name    string
		fs      fs.FS
		dirPath string
		want    openapi.Client
	}{{
		name: "fs nil and dir empty",
		want: &localSchemasClient{},
	}, {
		name:    "only dir",
		dirPath: "test",
		want: &localSchemasClient{
			dir: "test",
		},
	}, {
		name: "only fs",
		fs:   os.DirFS("."),
		want: &localSchemasClient{
			fs: os.DirFS("."),
		},
	}, {
		name:    "both fs and dir",
		fs:      os.DirFS("."),
		dirPath: "test",
		want: &localSchemasClient{
			fs:  os.DirFS("."),
			dir: "test",
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewLocalSchemaFiles(tt.fs, tt.dirPath); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewLocalSchemaFiles() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_localSchemasClient_Paths(t *testing.T) {
	tests := []struct {
		name    string
		fs      fs.FS
		dir     string
		want    sets.Set[string]
		wantErr bool
	}{{
		name: "fs nil and dir empty",
	}, {
		name: "only dir",
		dir:  "./builtins/1.27",
		want: sets.New(
			"api/v1",
			"apis/admissionregistration.k8s.io/v1",
			"apis/admissionregistration.k8s.io/v1alpha1",
			"apis/apiextensions.k8s.io/v1",
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
		name: "only fs",
		fs:   os.DirFS("./builtins/1.27"),
	}, {
		name: "both fs and dir",
		fs:   os.DirFS("./builtins"),
		dir:  "1.27",
		want: sets.New(
			"api/v1",
			"apis/admissionregistration.k8s.io/v1",
			"apis/admissionregistration.k8s.io/v1alpha1",
			"apis/apiextensions.k8s.io/v1",
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
		name: "invalid dir",
		dir:  "invalid",
		want: sets.New[string](),
	}, {
		name: "invalid fs",
		fs:   os.DirFS("../../invalid"),
		dir:  ".",
		want: sets.New[string](),
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := NewLocalSchemaFiles(tt.fs, tt.dir)
			paths, err := k.Paths()
			if (err != nil) != tt.wantErr {
				t.Errorf("localSchemasClient.Paths() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			var got sets.Set[string]
			if paths != nil {
				got = sets.New[string]()
				for key := range paths {
					got.Insert(key)
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("localSchemasClient.Paths() = %v, want %v", got, tt.want)
			}
		})
	}
}
