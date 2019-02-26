# cnat

The `cnat` (cloud native at) command extends Kubernetes to run a command at a certain point in time in the future, akin to the Linux [at](https://en.wikipedia.org/wiki/At_(command)) command.

Let's say you want to execute `echo yayk8s` at 2am on Friday, the 1st of March 2019. Here's what you would do (given the `cnat` CRD has been registered and the according operator is running):

```bash
$ cat runat.yaml
apiVersion: at.pk.kubernetes.sh/v1alpha1
kind: At
metadata:
  name: simplex
spec:
  schedule: 2019-03-01T02:00:00Z
  containers:
  - name: shell
    image: centos:7
    command:
    - "bin/bash"
    - "-c"
    - "echo yayk8s"

$ kubectl apply -f runat.yaml
at.pk.kubernetes.sh/simplex created

$ kubectl get at
NAME               AGE
simplex            20s
```

Note that the execution time (`schedule`) is given in [UTC](https://www.utctime.net/).