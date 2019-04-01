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

## Local

```bash
$ kubectl create ns cnat

$ kubectl -n cnat apply -f deploy/crds/

$ OPERATOR_NAME=cnatop operator-sdk up local --namespace "cnat"
```