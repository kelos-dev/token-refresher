# token-refresher

`token-refresher` is a small Go job that keeps a coding agent auth blob in sync inside Kubernetes.

The first provider is `codex`:

- reads `auth.json` from a Kubernetes `Secret`
- copies it into an isolated temporary `HOME`
- runs a lightweight `codex exec` to trigger validation or refresh
- reads the resulting `auth.json`
- updates the Kubernetes `Secret` only if the file changed

The runtime path is isolated on purpose. The refresher never mutates any existing host-side `~/.codex/auth.json` directly.

## Why this shape

Codex stores auth state in `~/.codex/auth.json`. For a Kubernetes `CronJob`, the cleanest flow is:

1. fetch the current `auth.json` from a secret
2. materialize it in a temp home directory
3. let `codex` operate against that copied state
4. write the updated file back to the secret if it changed

That makes the implementation safe for CronJobs and leaves room to add other agent providers later.

## Current defaults

- `CronJob` schedule: every 12 hours
- refresh behavior: every run performs a lightweight `codex exec`
- Kubernetes secret key: `auth.json`

## Configuration

Runtime configuration is intentionally small and uses flags instead of environment variables:

- `--agent-provider`: provider name. Currently `codex` only.
- `--secret-name`: name of the Kubernetes secret to read and update.

Everything else is fixed in code:

- namespace: inferred from the in-cluster service account namespace
- secret key: `auth.json`
- Codex command: `codex`
- refresh behavior: run on every scheduled execution
- timeout: `10m`

## Build

```bash
make build
make verify
make test
make image IMAGE_REPOSITORY=ghcr.io/kelos-dev/token-refresher VERSION=latest
```

The container image includes:

- the Go refresher binary
- Node.js
- `@openai/codex`

`make update` formats Go files and runs `go mod tidy`.

## Kubernetes setup

Create the starting secret from an existing local Codex auth file:

```bash
kubectl -n your-namespace create secret generic codex-auth \
  --from-file=auth.json=$HOME/.codex/auth.json
```

Apply RBAC and the `CronJob`:

```bash
kubectl -n your-namespace apply -f deploy/kubernetes/rbac.yaml
kubectl -n your-namespace apply -f deploy/kubernetes/cronjob.yaml
```

Before applying the `CronJob`, edit `deploy/kubernetes/cronjob.yaml` and set:

- `image`
- `--secret-name` if you use a different secret name

## Notes

- The job uses the in-cluster service account token and Kubernetes API directly. No `kubectl` dependency is required.
- The process updates the secret only when `auth.json` changes.
- The current implementation is deliberately provider-oriented so other agent auth formats can be added behind the same interface.
