package utils

import (
	"reflect"
	"testing"
)

func TestIsYaml(t *testing.T) {
	tests := []struct {
		name string
		file string
		want bool
	}{{
		name: "empty",
		file: "",
		want: false,
	}, {
		name: "with yaml",
		file: "test.yaml",
		want: true,
	}, {
		name: "with yml",
		file: "test.yml",
		want: true,
	}, {
		name: "with json",
		file: "test.json",
		want: false,
	}, {
		name: "with pdf",
		file: "test.pdf",
		want: false,
	}, {
		name: "without extension",
		file: "test",
		want: false,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsYaml(tt.file); got != tt.want {
				t.Errorf("IsYaml() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsJson(t *testing.T) {
	tests := []struct {
		name string
		file string
		want bool
	}{{
		name: "empty",
		file: "",
		want: false,
	}, {
		name: "with yaml",
		file: "test.yaml",
		want: false,
	}, {
		name: "with yml",
		file: "test.yml",
		want: false,
	}, {
		name: "with json",
		file: "test.json",
		want: true,
	}, {
		name: "with pdf",
		file: "test.pdf",
		want: false,
	}, {
		name: "without extension",
		file: "test",
		want: false,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsJson(tt.file); got != tt.want {
				t.Errorf("IsJson() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsYamlOrJson(t *testing.T) {
	tests := []struct {
		name string
		file string
		want bool
	}{{
		name: "empty",
		file: "",
		want: false,
	}, {
		name: "with yaml",
		file: "test.yaml",
		want: true,
	}, {
		name: "with yml",
		file: "test.yml",
		want: true,
	}, {
		name: "with json",
		file: "test.json",
		want: true,
	}, {
		name: "with pdf",
		file: "test.pdf",
		want: false,
	}, {
		name: "without extension",
		file: "test",
		want: false,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsYamlOrJson(tt.file); got != tt.want {
				t.Errorf("IsYamlOrJson() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindFiles(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    []string
		wantErr bool
	}{{
		name: "one folder",
		args: []string{
			"./testdata/a",
		},
		wantErr: false,
		want: []string{
			"testdata/a/a.json",
			"testdata/a/a.yaml",
			"testdata/a/a.yml",
		},
	}, {
		name: "two folders",
		args: []string{
			"./testdata/a",
			"./testdata/b",
		},
		wantErr: false,
		want: []string{
			"testdata/a/a.json",
			"testdata/a/a.yaml",
			"testdata/a/a.yml",
			"testdata/b/b.json",
			"testdata/b/b.yaml",
			"testdata/b/b.yml",
		},
	}, {
		name: "recursive",
		args: []string{
			"./testdata",
		},
		wantErr: false,
		want: []string{
			"testdata/a/a.json",
			"testdata/a/a.yaml",
			"testdata/a/a.yml",
			"testdata/b/b.json",
			"testdata/b/b.yaml",
			"testdata/b/b.yml",
		},
	}, {
		name: "invalid folder",
		args: []string{
			"./testdata/c",
		},
		wantErr: true,
	}, {
		name: "one pdf",
		args: []string{
			"./testdata/a/a.pdf",
		},
		wantErr: false,
		want: []string{
			"./testdata/a/a.pdf",
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FindFiles(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Errorf("FindFiles() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FindFiles() = %v, want %v", got, tt.want)
			}
		})
	}
}
