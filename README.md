# kubectl-validate

kubectl-validate is a SIG-CLI subproject to support the local validation of 
resources for native Kubernetes types and CRDs.

This project has two goals:

1.) Shift-left validation of resources with as close to parity to server-side
Kubernetes as possible.
2.) Improve declarative validation support in upstream Kubernetes over time,
making those improvements available for kubectl-validate users early.

## Comparison With Other Tools

`kubectl-validate` distinguishes itself among other OpenAPI-based validators by
to its deep integration with Kubernetes upstream validation. `kubectl-validate`
is written by Kubernetes apiserver authors using the same code as the server-side.

This allows `kubectl-validate` to give the most accurate error messages and support
the Kubernetes-specific validations often ignored by other tools.

## Drawbacks

`kubectl-validate` suffers from similar drawbacks to other OpenAPI-based validation tools.
For CRDs, users should expect a first-class experience likely identical to how the server treats CRDs.

For Native Types, the OpenAPI definitions are a best-effort replication of the
handwritten validation rules conducted by the apiserver. There is an ongoing 
effort to improve the quality of native types' definitions, and eventually replace 
the handwritten rules. As the improvements are made upstream, kubectl-validate 
will integrate them to make them available to all users.

Bottom line is that users should expect nearly a seamless experience with CRDs,
and great but constantly improving support for the built-in Kubernetes types.

# Installation

You can get started using kubectl-validate right away with an existing installation
of Go:

```sh
go install sigs.k8s.io/kubectl-validate@latest
```

# Usage

Once installed, you have the option of invoking `kubectl-validate` through
`kubectl` or directly:

## Example

```sh
kubectl validate ./path/to/file.yaml
```

is equivalent to 

```sh
kubectl-validate ./path/to/file.yaml
```

## Native Types

Native types can be validated out of the box with `kubectl-validate`. The tool
has built-in schemas for Kubernetes 1.23-1.27 which are kept up to date with releases.

By default, the tool will validate native types with the latest built-in version it
ships with. You can specify a specific Kubernetes version to validate against 
using the version argument:

```sh
kubectl validate ./my_pod.yaml --version 1.27
```

If the version is not recognized, `kubectl-validate` will attempt to look up
the schemas for the selected version in the official upstream Kubernetes repository
on GitHub.

## CRD

`kubectl-validate` is also capable of validating CRDs. To do that, it needs to be
aware of their definitions. There are two ways to provide this:

### Cluster-Installed CRDs

If you have access to a cluster with the CRDs already installed, then kubectl-validate
will automatically connect to the cluster to download the definitions.

This is done automatically if your cluster is the currently active context,
or you can supply the name of the context for the cluster you'd like to use:

```sh
kubectl validate ./my_crd.yaml --context <cluster_context>
```

### Local CRDs

If you are working offline or do not have access to a cluster with the CRDs installed,
you may supply a directory to use as a search path for types:

```sh
kubectl validate ./my_crd.yaml --local-crds ./path/to/folder/with/crds
```

### Local openapi schemas

If you are working offline or do not have access to a cluster to load openapi schemas,
you may supply a directory to use as a search path for schemas:

```sh
kubectl validate ./my_crd.yaml --local-schemas ./path/to/folder/with/schemas
```

Directory should have openapi files following directory layout:
```sh
/<apis>/<group>/<version>.json
/api/<version>.json
```

## JSON Output

By default the output of the tool is human readable, but you may also
request JSON structured output for easier integration with other software:

```sh
kubectl-validate ./testcases/error_array_instead_of_map.yaml --output json
```

Example output:
```json
{
    "./testcases/error_array_instead_of_map.yaml": {
        "metadata": {},
        "status": "Failure",
        "message": "ConfigMap.core \"my-deployment\" is invalid: data: Invalid value: \"array\": data in body must be of type object: \"array\"",
        "reason": "Invalid",
        "details": {
            "name": "my-deployment",
            "group": "core",
            "kind": "ConfigMap",
            "causes": [
                {
                    "reason": "FieldValueTypeInvalid",
                    "message": "Invalid value: \"array\": data in body must be of type object: \"array\"",
                    "field": "data"
                }
            ]
        },
        "code": 422
    }
}
```

# Usage in CI Systems

> 🚧 COMING SOON: native docker image & GitHub action 🚧

## GitHub Actions workflows

Here is an example of a simaple GitHub Actions workflow that uses `kubectl-validate` to validate Kubernetes manifests.
This workflow will run on every pull request to the `main` branch and will fail if any of the manifests in the dir `k8s-manifest/` are invalid.

```yaml
name: kubectl-validate
on:
  pull_request:
    branches:
      - main
  
jobs:
  k8sManifestsValidation:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout repo content
        uses: actions/checkout@v3
        
      - name: Setup go
        uses: actions/setup-go@v4
        with:
          go-version: '1.20'
          
      - name: Install kubectl-validate
        run: go install sigs.k8s.io/kubectl-validate@latest
        
      - name: Run kubectl-validate
        run: kubectl-validate ./k8s-manifest/ --version 1.23
```

## Docker

This project doesn't have a native docker image (yet), but you can use the given Dockerfile to build one.

First, you will need to build it from the Dockerfile (final image size is 98MB):

```sh
docker build -t kubectl-validate .
```

And then you can run it and mount the directory (`k8s-manifest`) with your manifests:

```sh
docker run --volume k8s-manifest:/usr/local/k8s-manifest -it kubectl-validate --version 1.23 /usr/local/k8s-manifest/
``` 

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- [Kubernetes Slack](https://slack.k8s.io/) in the [#kubectl-validate](https://kubernetes.slack.com/archives/C057WPL56BS) channel.
- [Mailing List](https://groups.google.com/a/kubernetes.io/g/dev)

### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).

[owners]: https://git.k8s.io/community/contributors/guide/owners.md
[Creative Commons 4.0]: https://git.k8s.io/website/LICENSE
