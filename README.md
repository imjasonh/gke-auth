# Auth to GKE without gcloud

### Installatioin

```
go install github.com/imjasonh/gke-auth@latest
```

### Use

```
gke-auth --project=[MY_PROJECT] \
    --location=[REGION_OR_ZONE] \
    --cluster=[CLUSTER_NAME]
```

Now you have auth and kubeconfigs set up to use the cluster.

Test it with:

```
kubectl get pods
```
