# Development

Install [Kubebuilder](https://book.kubebuilder.io/quick_start.html) and then bootstrap the `At` operator as follows:

```bash
$ mkdir cnat-kubebuilder && cd $_

$ kubebuilder init \
              --domain kubernetes.sh \
              --license apache2 \
              --owner "We, the Kube people"

$ kubebuilder create api \
              --group cnat \
              --version v1alpha1 \
              --kind At
Create Resource under pkg/apis [y/n]?
y
Create Controller under pkg/controller [y/n]?
y
...
```

## Launch operator locally

```bash
# install CRD via:
$ make install

# create the dedicated namespace and set the context to it:
$ kubectl create ns cnat && \
  kubectl config set-context $(kubectl config current-context) --namespace=cnat

# launch operator:
$ make run
```

Now, once we create the `At` custom resource, we see the execution at the scheduled time:

```bash
$ kubectl apply -f config/samples/cnat_v1alpha1_at.yaml

$ kubectl -n cnat  get at,po
NAME                               AGE
at.cnat.kubernetes.sh/example-at   54s

NAME                 READY   STATUS        RESTARTS   AGE
pod/example-at-pod   0/1     Completed     2          56s
```

## Implement business logic

* In [at_types.go](cnat-operator/pkg/apis/cnat/v1alpha1/at_types.go): we modify the `AtSpec` struct to include the respective fields such as `schedule` and `command` and use `operator-sdk generate k8s` to regenerate code (and `operator-sdk generate openapi` for the OpenAPI bits).
* In [at_controller.go](cnat-operator/pkg/controller/at/at_controller.go): we modify the `Reconcile(request reconcile.Request)` method to create a pod at the time defined in `Spec.Schedule`.