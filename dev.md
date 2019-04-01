# Development

Install the [Operator SDK](https://github.com/operator-framework/operator-sdk#prerequisites) and then bootstrap the `At` operator as follows:

```bash
$ operator-sdk new cnat-operator && cd cnat-operator

$ operator-sdk add api \
               --api-version=cnat.kubernetes.sh/v1alpha1 \
               --kind=At

$ operator-sdk add controller \
               --api-version=cnat.kubernetes.sh/v1alpha1 \
               --kind=At  
```

## Launch operator locally

```bash
$ kubectl create ns cnat

$ kubectl -n cnat apply -f deploy/crds/

$ OPERATOR_NAME=cnatop operator-sdk up local --namespace "cnat"
```

## Implement business logic

* In [at_types.go](cnat-operator/pkg/apis/cnat/v1alpha1/at_types.go): we modify the `AtSpec` struct to include the respective fields and use `operator-sdk generate k8s` to regenerate code.
* In [at_controller.go](cnat-operator/pkg/controller/at/at_controller.go): we modify the `Reconcile(request reconcile.Request)` method to create a pod at the time defined in `Spec.Schedule`.