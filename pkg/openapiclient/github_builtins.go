package openapiclient

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"k8s.io/client-go/openapi"
	"sigs.k8s.io/kubectl-validate/pkg/openapiclient/groupversion"
)

// client which sources openapi definitions from GitHub
type githubBuiltins struct {
	version string
}

type ghResponseObject struct {
	Name         string `json:"name"`
	RelativePath string `json:"path"`
	DownloadURI  string `json:"download_url"`
	Type         string `json:"type"`
}

func NewGitHubBuiltins(k8sVersion string) openapi.Client {
	return githubBuiltins{
		version: k8sVersion,
	}
}

func (g githubBuiltins) Paths() (map[string]openapi.GroupVersion, error) {
	if len(g.version) == 0 {
		return nil, nil
	}

	// xh "https://api.github.com/repos/kubernetes/kubernetes/contents/api/openapi-spec/v3?ref=release-1.27" Accept:"application/vnd.github+json"
	//TODO: responses use and respect ETAG. use a disk cache
	ghResponse, err := http.Get(fmt.Sprintf("https://api.github.com/repos/kubernetes/kubernetes/contents/api/openapi-spec/v3?ref=release-%v", g.version))
	if err != nil {
		return nil, fmt.Errorf("error retreiving specs from GitHub: %w", err)
	}
	defer ghResponse.Body.Close()
	ghBody, err := io.ReadAll(ghResponse.Body)
	if err != nil {
		return nil, fmt.Errorf("error downloading specs from GitHub: %w", err)
	}

	if ghResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download GitHub spec for version '%v': %v", g.version, string(ghBody))
	}

	var decodedResponse []ghResponseObject
	if err := json.Unmarshal(ghBody, &decodedResponse); err != nil {
		return nil, fmt.Errorf("failed to parse github response: %w", err)
	}

	// filter out files in the folder for only ones that match the pattern we
	// know about
	res := map[string]openapi.GroupVersion{}
	suf := "_openapi.json"
	pre1 := "apis__"
	pre2 := "api__"
	for _, f := range decodedResponse {
		if !strings.HasSuffix(f.Name, suf) {
			continue
		} else if !strings.HasPrefix(f.Name, pre1) && !strings.HasPrefix(f.Name, pre2) {
			continue
		}

		trimmed := strings.TrimSuffix(f.Name, suf)
		trimmed = strings.TrimPrefix(trimmed, pre1)
		trimmed = strings.TrimPrefix(trimmed, pre2)

		group, version, hasVersion := strings.Cut(trimmed, "__")
		if !hasVersion {
			if strings.HasPrefix(f.Name, pre2) {
				version = group
				group = ""
			} else {
				continue
			}
		} else if len(f.DownloadURI) == 0 {
			continue
		} else if f.Type != "file" {
			continue
		}

		key := "apis/" + group + "/" + version
		if len(group) == 0 {
			key = "api/" + version
		}
		res[key] = groupversion.NewForHttp(f.DownloadURI)
	}
	return res, nil
}
