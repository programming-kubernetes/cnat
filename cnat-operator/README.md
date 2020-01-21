# Development

Install the [Operator SDK](https://github.com/operator-framework/operator-sdk#prerequisites) and then bootstrap the `At` operator as follows:

```bash
$ operator-sdk new cnat-operator && cd cnat-operator

$ operator-sdk add api \
               --api-version=cnat.programming-kubernetes.info/v1alpha1 \
               --kind=At

$ operator-sdk add controller \
               --api-version=cnat.programming-kubernetes.info/v1alpha1 \
               --kind=At  
```

## Launch operator locally

```bash
$ kubectl apply -f deploy/crds/cnat_v1alpha1_at_crd.yaml

$ kubectl create ns cnat

$ OPERATOR_NAME=cnatop operator-sdk up local --namespace "cnat"
```

Now, once we create the `At` custom resource, we see the execution at the scheduled time:

```bash
$ kubectl -n cnat apply -f deploy/crds/cnat_v1alpha1_at_cr.yaml

$ kubectl -n cnat  get at,po
NAME                               AGE
at.cnat.programming-kubernetes.info/example-at   54s

NAME                 READY   STATUS             RESTARTS   AGE
pod/example-at-pod   0/1     CrashLoopBackOff   2          34s
```

## Implement business logic

* In [at_types.go](/cnat-operator/pkg/apis/cnat/v1alpha1/at_types.go): we modify the `AtSpec` struct to include the respective fields such as `schedule` and `command` and use `operator-sdk generate k8s` to regenerate code (and `operator-sdk generate openapi` for the OpenAPI bits).
* In [at_controller.go](/cnat-operator/pkg/controller/at/at_controller.go): we modify the `Reconcile(request reconcile.Request)` method to create a pod at the time defined in `Spec.Schedule`.
