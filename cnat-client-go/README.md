# Development

We follow the [sample-controller](https://github.com/kubernetes/sample-controller)
along with the [client-go library](https://github.com/kubernetes/client-go).

It makes use of the generators in [k8s.io/code-generator](https://github.com/kubernetes/code-generator) to generate a typed client, informers, listers and deep-copy functions. You can do this yourself using the `./hack/update-codegen.sh` script.

The `update-codegen` script will automatically generate the following:

* `pkg/apis/samplecontroller/v1alpha1/zz_generated.deepcopy.go`
* `pkg/generated/`

Changes should not be made to these files manually, and when creating your own
controller based off of this implementation you should not copy these files and
instead run the `update-codegen` script to generate your own.

```bash
# build custom controller binary:
$ go build -o cnat-controller .

# launch custom controller locally:
$ ./cnat-controller -kubeconfig=$HOME/.kube/config

# after API/CRD changes:
$ ./hack/update-codegen.sh

# register At custom resource definition:
$ kubectl apply -f artifacts/examples/cnat-crd.yaml

# create an At custom resource:
$ kubectl apply -f artifacts/examples/cnat-example.yaml
```
