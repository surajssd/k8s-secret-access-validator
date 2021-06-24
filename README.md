# Kubernetes Secret Access Validator

This is a Kubernetes validating webhook which checks if a user is requesting access to secret covertly. A user can access a secret even if they don't have access to it by creating a pod and mounting that secret in the pod. All they have to know is secret name.

This webhook server checks the incoming pod requests and verifies if the user creating the pod has RBAC access to the specified secret. If the user does not have RBAC access to the secret then the request is denied.

## Prerequisite

- A Kubernetes cluster with Kubectl access to it.
- [Helm installed](https://helm.sh/docs/intro/install/).

## Install

```bash
cd config
helm repo update
helm dependency update
helm install validate-secrets \
    --create-namespace \
    --namespace validate-secrets \
    .
```

## Trying it out

We will try to recreate the scenario explained in [this blog post](https://suraj.io/post/2021/05/access-k8s-secrets/), which shows how a user can access any secret even if they don't have RBAC permission to access it.

Once you install the above webhook server, create a test user that has access to pod, but not to secrets.

```bash
kubectl -n default create role pod-all --verb=* --resource=pods --resource=pods/exec
kubectl -n default create rolebinding pod-all:nastyuser --role=pod-all --user=nastyuser
kubectl -n default create secret generic supersecret --from-literal data=supersecretvaluesinhere
alias kubectl='kubectl -n default --as=nastyuser'
kubectl -n default auth can-i --list
```

Save this pod config in `pod.yaml` and we will try to deploy it:

```yaml
apiVersion: v1
kind: Pod
metadata:
  labels:
    run: sleep
  name: sleep
spec:
  containers:
  - args:
    - sleep
    - infinity
    image: fedora
    name: sleep
    resources: {}
    volumeMounts:
    - name: hackedsecret
      mountPath: /access-secret/
  dnsPolicy: ClusterFirst
  restartPolicy: Always
  volumes:
  - name: hackedsecret
    secret:
      secretName: supersecret
```

Try to deploy it:

```console
$ kubectl create -f pod.yaml
Error from server: error when creating "pod.yaml": admission webhook "validating.suraj.io" denied the request: User "nastyuser" does not have access to the secret "supersecret" in the namespace "default".
```

It failed with an error saying that the user cannot deploy this particular pod.

## Upgrade

```bash
helm upgrade validate-secrets \
    --namespace validate-secrets \
    .
```


## Uninstall

```bash
helm uninstall validate-secrets -n validate-secrets
helm template validate-secrets . | kubectl delete -f -
kubectl delete ns validate-secrets
```

## Missing features

- Add Readiness and Liveness probes.
- Add support for checking the group permissions.
- Add unit tests.
- Add support for Deployment, StatefulSets, Job, CronJob, Daemonset, etc. or any in built controller type that has pod spec in it.
- Create a library that anyone can import it in their webhook server.
