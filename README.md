# Auth to GKE without gcloud

### Installation

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

Or, using GitHub Actions:

```yaml
- uses: imjasonh/gke-auth@v0.0.2
  with:
    project: [MY_PROJECT]
    location: [REGION_OR_ZONE]
    cluster: [CLUSTER_NAME]
```

You probably want to [set up Workload Identity](https://github.com/google-github-actions/auth#usage) between your GitHub Actions workflow and your GCP project.

Test it with:

```
kubectl get pods
```

### Why?

`gcloud` is great.
It's like a Swiss army knife for the cloud, if a knife could do anything to a cloud.

It does so much.
_It does soooo much!_

Too much.

This leads it to be really huge.
Hundreds of megabytes of sweet delicious Python.
Python that has to be interpreted before it can even start running anything.

If you're downloading and installing and running `gcloud` just to execute `gcloud container clusters get-credentials` -- _especially_ in a CI environment -- you're wasting a lot of time.

Installing `gcloud` can take _minutes_, compared to just a few seconds with `gke-auth`, even if you have to build it from source.
