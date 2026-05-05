# README demo

This directory contains the reproducible recording used by the repository
README.

```console
$ ./_tools/readme-demo/render.sh
```

The script builds `kube-prompt`, creates a disposable KWOK cluster, seeds demo
Kubernetes resources, renders `assets/kube-prompt-demo.gif` with VHS, and then
deletes the cluster.

Required tools:

- Go: `$HOME/.tools/go/1.26.2/bin/go`
- kubectl: `$HOME/.tools/kubectl/1.36.0/kubectl`
- kwokctl: `$HOME/.tools/kwokctl/0.7.0/kwokctl`
- Docker

The renderer uses `ghcr.io/charmbracelet/vhs:v0.11.0`, which includes the VHS
runtime dependencies.
