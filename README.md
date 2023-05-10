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
For CRDs, users should expect a first class experience likely identical to how the server treats CRDs.

For Native Types, the OpenAPI definitions are a best-effort replication of the
handwritten validation rules conducted by the apiserver. There is an ongoing 
effort to improve the quality of native type definitions, and eventually replace 
the handwritten rules. As the improvements are made upstream, kubectl-validate 
will integrate them to make them available to all users.

Bottom line is that users should expect nearly a seamless experience with CRDs,
and great but constantly improving support for the builtin Kubernetes types.

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

By default the tool will validate native types with the latest builtin version it
ships with. You can specify a specify Kubernetes version to validate against 
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

> 🚧 COMING SOON 🚧

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- [Slack](https://slack.k8s.io/)
- [Mailing List](https://groups.google.com/a/kubernetes.io/g/dev)

### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).

[owners]: https://git.k8s.io/community/contributors/guide/owners.md
[Creative Commons 4.0]: https://git.k8s.io/website/LICENSE
