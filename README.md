# cnat

The `cnat` (cloud native at) command extends Kubernetes to run a command at a certain point in time in the future, akin to the Linux [at](https://en.wikipedia.org/wiki/At_(command)) command.

Let's say you want to execute `echo YAY` at 2am on 3rd July 2019. Here's what you would do (given the `cnat` CRD has been registered and the according operator is running):

```bash
$ cat runat.yaml
apiVersion: cnat.programming-kubernetes.info/v1alpha1
kind: At
metadata:
  name: example-at
spec:
  schedule: "2019-07-03T02:00:00Z"
  command: "echo YAY"


$ kubectl apply -f runat.yaml
cnat.programming-kubernetes.info/example-at created

$ kubectl get at
NAME               AGE
example-at         20s

$ kubectl logs example-at-pod
YAY
```

Note that the execution time (`schedule`) is given in [UTC](https://www.utctime.net/).

In order to use the cnat custom resource, you have to run the respective custom controller, implemented here in three different ways:

* Following the `sample-controller` in [cnat-client-go](cnat-client-go/)
* Using Kubebuilder in [cnat-kubebuilder](cnat-kubebuilder/)
* Using the Operator SDK in [cnat-operator](cnat-operator/)
