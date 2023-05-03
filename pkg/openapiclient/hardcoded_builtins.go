package openapiclient

import (
	"embed"
	"fmt"
	"path/filepath"
	"strings"

	"k8s.io/client-go/openapi"
)

//go:embed builtins
var hardcodedBuiltins embed.FS

// client which provides hardcoded openapi for known k8s versions
type hardcodedResolver struct {
	version string
}

func NewHardcodedBuiltins(version string) openapi.Client {
	return hardcodedResolver{version: version}
}

func (k hardcodedResolver) Paths() (map[string]openapi.GroupVersion, error) {
	if len(k.version) == 0 {
		return nil, nil
	}

	allVersions, err := hardcodedBuiltins.ReadDir("builtins")
	if err != nil {
		return nil, err
	}

	for _, v := range allVersions {
		if v.Name() == k.version {
			res := map[string]openapi.GroupVersion{}

			apiDir := filepath.Join("builtins", v.Name(), "api")
			apiListing, _ := hardcodedBuiltins.ReadDir(apiDir)
			for _, v := range apiListing {
				// chop extension
				ext := filepath.Ext(v.Name())
				version := strings.TrimSuffix(v.Name(), ext)
				res[fmt.Sprintf("api/%s", version)] = localGroupVersion{fs: &hardcodedBuiltins, filepath: filepath.Join(apiDir, v.Name())}
			}

			apisDir := filepath.Join("builtins", v.Name(), "apis")
			apisListing, _ := hardcodedBuiltins.ReadDir(apisDir)
			for _, g := range apisListing {
				gDir := filepath.Join(apisDir, g.Name())
				vs, err := hardcodedBuiltins.ReadDir(gDir)
				if err != nil {
					return nil, err
				}

				for _, v := range vs {
					// chop extension
					ext := filepath.Ext(v.Name())
					version := strings.TrimSuffix(v.Name(), ext)
					res[fmt.Sprintf("apis/%s/%s", g.Name(), version)] = localGroupVersion{fs: &hardcodedBuiltins, filepath: filepath.Join(gDir, v.Name())}
				}
			}

			return res, nil
		}
	}

	return nil, fmt.Errorf("couldn't find hardcoded schemas for version %s", k.version)
}
