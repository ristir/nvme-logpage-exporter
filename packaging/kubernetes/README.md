# Kubernetes DaemonSet

```bash
kubectl label node <node> nvme=true
kubectl apply -f daemonset.yaml
```

The manifest tracks `:latest`, which also makes Kubernetes default the pull
policy to `Always`, so a `kubectl rollout restart` picks up a new release.
Pin a version tag instead if you would rather choose when that happens —
note that doing so flips the default to `IfNotPresent`.

The node label is what keeps a pod off every diskless VM in the cluster.
Drop the `affinity` block to run everywhere instead; nodes without NVMe
export nothing and cost one idle pod each.

## Why this one needs `privileged: true`

Everywhere else — systemd, plain Docker — `CAP_SYS_ADMIN` plus access to
the controller node is enough, and `--privileged` is not needed. Kubernetes
is the exception, for a reason worth stating plainly because it is easy to
assume otherwise:

Docker's `--device` does two things: it creates the node inside the
container **and** writes a device cgroup rule permitting it. Kubernetes has
no field for the second half. A `hostPath` mount of `/dev` gives the pod
the node and nothing else, so `open("/dev/nvme0")` returns `EPERM` no
matter how many capabilities are added.

Measured on a live host, same image, same capability, only the device
handling differing:

```
docker run --cap-add=SYS_ADMIN --device=/dev/nvme0 …   → works
docker run --cap-add=SYS_ADMIN -v /dev:/dev      …   → EPERM on open
```

The alternative to `privileged` is a device plugin advertising the
controller nodes as a resource. That is a second component to run and
upgrade; if your cluster already has one, use it and drop `privileged`.

## Scraping

The pod annotations suit a Prometheus configured with the usual pod
discovery. With Prometheus Operator, add a `PodMonitor` selecting
`app.kubernetes.io/name: nvme-logpage-exporter` on port `metrics` instead.

`hostNetwork: true` and `hostPort: 9683` make the endpoint reachable at the
node address, which is what a Consul or static scrape config expects. Drop
both if pod-IP discovery is enough — nothing in the exporter needs the host
network itself.
