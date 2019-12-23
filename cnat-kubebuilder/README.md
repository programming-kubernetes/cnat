# Development

Install [Kubebuilder](https://book.kubebuilder.io/quick-start.html) and then bootstrap the `At` operator as follows:

```bash
$ mkdir cnat-kubebuilder && cd $_

$ kubebuilder init \
              --domain programming-kubernetes.info \
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
at.cnat.programming-kubernetes.info/example-at   54s

NAME                 READY   STATUS        RESTARTS   AGE
pod/example-at-pod   0/1     Completed     0          32s
```

## Implement business logic

* In [at_types.go](pkg/apis/cnat/v1alpha1/at_types.go): we modify the `AtSpec` struct to include the respective fields such as `schedule` and `command`. Note that you must run `make` whenever you change something here in order to regenerate the controller code. 
* In [at_controller.go](pkg/controller/at/at_controller.go): we modify the `Reconcile(request reconcile.Request)` method to create a pod at the time defined in `Spec.Schedule`.
