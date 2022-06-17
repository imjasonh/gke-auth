# Auth to GKE without gcloud

[![Test](https://github.com/imjasonh/gke-auth/actions/workflows/test-action.yaml/badge.svg)](https://github.com/imjasonh/gke-auth/actions/workflows/test-action.yaml)

### Installation

```
go install github.com/imjasonh/gke-auth@latest
```

### Use

Once you've installed the Go binary, run it:

```
gke-auth --project=[MY_PROJECT] \
    --location=[REGION_OR_ZONE] \
    --cluster=[CLUSTER_NAME]
```

Now you have auth and kubeconfigs set up to use the cluster.

Or, using GitHub Actions:

```yaml
- uses: imjasonh/gke-auth@v0.1.0
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

If you're downloading and installing and running `gcloud` just to execute `gcloud container clusters get-credentials` so you can switch to using `kubectl` -- _especially_ in a CI environment -- you're wasting a lot of time.

Installing `gcloud` can take _minutes_, compared to just a few seconds with `gke-auth`, even if you have to build it from source.

The [example GitHub Actions workflow](./.github/workflows/test-action.yaml) sets up Workload Identity auth to GKE and lists pods in a cluster in _about seven seconds_.
Compare that to [the equivalent using `gcloud`](./.github/workflows/using-gcloud.yaml), which takes 33 seconds.

### `gke-gcloud-auth-plugin`

`gcloud` itself includes a K8s auth plugin very similar to `gke-auth`, called [`gke-gcloud-auth-plugin`](https://github.com/kubernetes/cloud-provider-gcp/tree/master/cmd/gke-gcloud-auth-plugin).

For more information about this plugin, see this [GCP blog post](https://cloud.google.com/blog/products/containers-kubernetes/kubectl-auth-changes-in-gke).

Like `gke-auth`, this plugin can be configured to be used by `kubectl` and `client-go` to get auth credentials.

Unlike `gke-auth`, the plugin is installed using `gcloud components add gke-gcloud-auth-plugin`, and configured to be used with `gcloud container get-credentials`, so while it's still better than having `kubectl` invoke `gcloud` every time it needs new credentials, it still seems to mostly require `gcloud` to install and use it, at least as recommended in documentation.

`gke-auth` should be more or less equivalent to `gke-gcloud-auth-plugin` in core functionality, but `gke-auth` doesn't need or recommend `gcloud` at all to install or configure it.
