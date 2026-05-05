#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TOOLS=${CODEX_TOOLS:-"$HOME/.tools"}
GO_BIN=${GO_BIN:-"$TOOLS/go/1.26.2/bin/go"}
KUBECTL_BIN=${KUBECTL_BIN:-"$TOOLS/kubectl/1.36.0/kubectl"}
KWOKCTL_BIN=${KWOKCTL_BIN:-"$TOOLS/kwokctl/0.7.0/kwokctl"}
VHS_IMAGE=${VHS_IMAGE:-"ghcr.io/charmbracelet/vhs:v0.11.0"}
CLUSTER_NAME=${CLUSTER_NAME:-"kube-prompt-readme-demo-$$"}
OUTPUT="$ROOT/assets/kube-prompt-demo.gif"

PATH="$TOOLS/bin:$PATH"
export GOCACHE=${GOCACHE:-"$HOME/.cache/go-build"}
export GOPATH=${GOPATH:-"$HOME/go"}
export GOMODCACHE=${GOMODCACHE:-"$HOME/go/pkg/mod"}

need_file() {
  if [ ! -x "$1" ]; then
    echo "required executable not found or not executable: $1" >&2
    exit 1
  fi
}

need_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "required command not found on PATH: $1" >&2
    exit 1
  fi
}

need_file "$GO_BIN"
need_file "$KUBECTL_BIN"
need_file "$KWOKCTL_BIN"
need_command docker

tmp=$(mktemp -d)
cleanup() {
  "$KWOKCTL_BIN" delete cluster --name "$CLUSTER_NAME" >/dev/null 2>&1 || true
  rm -rf "$tmp"
}
trap cleanup EXIT

mkdir -p "$ROOT/assets" "$GOCACHE" "$GOPATH" "$GOMODCACHE"

echo "Building kube-prompt demo binary..."
revision=$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)
(
  cd "$ROOT"
  CGO_ENABLED=0 GO111MODULE=on GOPROXY=https://proxy.golang.org,direct \
    "$GO_BIN" build -ldflags "-X main.version=demo -X main.revision=$revision" \
    -o "$tmp/kube-prompt" .
)

echo "Creating KWOK cluster $CLUSTER_NAME..."
"$KWOKCTL_BIN" delete cluster --name "$CLUSTER_NAME" >/dev/null 2>&1 || true
"$KWOKCTL_BIN" create cluster \
  --name "$CLUSTER_NAME" \
  --runtime docker \
  --kubeconfig "$tmp/kubeconfig" \
  --wait 120s \
  --quiet-pull

cat > "$tmp/base.yaml" <<'YAML'
apiVersion: v1
kind: Namespace
metadata:
  name: apps
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: apps
spec:
  replicas: 0
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
      - name: web
        image: nginx:1.27
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: db
  namespace: apps
spec:
  serviceName: db
  replicas: 0
  selector:
    matchLabels:
      app: db
  template:
    metadata:
      labels:
        app: db
    spec:
      containers:
      - name: db
        image: postgres:16
---
apiVersion: v1
kind: Pod
metadata:
  name: web-5f7b4d8d9c-2spbr
  namespace: apps
  labels:
    app: web
spec:
  containers:
  - name: web
    image: nginx:1.27
---
apiVersion: v1
kind: Pod
metadata:
  name: web-5f7b4d8d9c-7kq2x
  namespace: apps
  labels:
    app: web
spec:
  containers:
  - name: web
    image: nginx:1.27
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.example.com
spec:
  group: example.com
  names:
    kind: Widget
    plural: widgets
    singular: widget
    shortNames:
    - wdg
  scope: Namespaced
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            x-kubernetes-preserve-unknown-fields: true
YAML

cat > "$tmp/widgets.yaml" <<'YAML'
apiVersion: example.com/v1
kind: Widget
metadata:
  name: alpha
  namespace: apps
spec:
  size: small
---
apiVersion: example.com/v1
kind: Widget
metadata:
  name: bravo
  namespace: apps
spec:
  size: medium
YAML

echo "Seeding demo resources..."
"$KUBECTL_BIN" --kubeconfig "$tmp/kubeconfig" apply -f "$tmp/base.yaml" >/dev/null
"$KUBECTL_BIN" --kubeconfig "$tmp/kubeconfig" wait \
  --for=condition=Established crd/widgets.example.com --timeout=60s >/dev/null
"$KUBECTL_BIN" --kubeconfig "$tmp/kubeconfig" apply -f "$tmp/widgets.yaml" >/dev/null

pod_status_patch=$(cat <<'JSON'
{
  "status": {
    "phase": "Running",
    "conditions": [
      {"type": "PodScheduled", "status": "True"},
      {"type": "Initialized", "status": "True"},
      {"type": "ContainersReady", "status": "True"},
      {"type": "Ready", "status": "True"}
    ],
    "containerStatuses": [
      {
        "name": "web",
        "ready": true,
        "restartCount": 0,
        "image": "nginx:1.27",
        "imageID": "demo",
        "containerID": "containerd://demo",
        "started": true,
        "state": {
          "running": {
            "startedAt": "2026-01-01T00:00:00Z"
          }
        }
      }
    ]
  }
}
JSON
)

for pod in web-5f7b4d8d9c-2spbr web-5f7b4d8d9c-7kq2x; do
  "$KUBECTL_BIN" --kubeconfig "$tmp/kubeconfig" -n apps patch pod "$pod" \
    --subresource=status \
    --type=merge \
    -p "$pod_status_patch" \
    >/dev/null
done

"$KUBECTL_BIN" --kubeconfig "$tmp/kubeconfig" config view --flatten --minify > "$tmp/kubeconfig.flat"
mv "$tmp/kubeconfig.flat" "$tmp/kubeconfig"

echo "Rendering $OUTPUT..."
docker pull "$VHS_IMAGE" >/dev/null
docker run --rm \
  --network host \
  --user "$(id -u):$(id -g)" \
  -e HOME=/tmp \
  -v "$ROOT:/vhs" \
  -v "$tmp:/demo:ro" \
  -v "$tmp/kube-prompt:/usr/local/bin/kube-prompt:ro" \
  -v "$KUBECTL_BIN:/usr/local/bin/kubectl:ro" \
  "$VHS_IMAGE" /vhs/_tools/readme-demo/kube-prompt-demo.tape

ls -lh "$OUTPUT"
