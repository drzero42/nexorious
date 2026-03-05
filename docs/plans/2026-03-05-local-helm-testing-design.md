# Local Helm Chart Testing — Design

**Date:** 2026-03-05

## Problem

There is no way to test the Helm chart locally end-to-end. `helm lint` and `helm template` validate syntax but cannot catch runtime issues (PVC provisioning, ingress wiring, pod startup, health probes).

## Decision

Use **minikube with the KVM2 driver** for local Kubernetes cluster testing.

- **KVM2 over Podman driver**: Podman is the only container runtime available. The KVM2 driver provides a real VM with stable networking, making `minikube tunnel` and ingress reliable — the Podman rootless driver has known quirks with ingress/LoadBalancer access that would undermine the testing goal.
- **minikube over kind**: minikube ships built-in addons for both storage provisioning and nginx ingress, eliminating manual setup steps that kind would require.

## Files

| File | Status | Purpose |
|------|--------|---------|
| `Taskfile.yml` | New | Task runner at repo root |
| `deploy/helm/values-dev.yaml` | New | Committed dev overrides (no secrets) |
| `.env.example` | New | Root-level env template (currently missing) |

## Tasks

| Task | Description |
|------|-------------|
| `task dev:up` | Full setup: start cluster → enable addons → deploy chart (default `TAG=dev`) |
| `task dev:up TAG=1.2.3` | Same with a specific image tag |
| `task dev:deploy` | Deploy/upgrade Helm chart only (cluster already running) |
| `task dev:down` | Uninstall Helm release + stop minikube |
| `minikube:start` | Start minikube with kvm2 driver, 4 CPUs, 8 GB RAM |
| `minikube:addons` | Enable `storage-provisioner` and `ingress` addons |

An internal `check-env` task runs before any deploy step. It exits with a clear error message if `.env` is missing or if `IGDB_CLIENT_ID` / `IGDB_CLIENT_SECRET` are absent.

## Helm Deploy Strategy

- Base chart values come from `deploy/helm/values.yaml` (existing).
- Dev overrides are layered on via `-f deploy/helm/values-dev.yaml` (new, committed).
- IGDB credentials are parsed from the root `.env` file and passed as `--set` flags.
- Image tag is passed as `--set` for all three image controllers (api, worker, scheduler).

### `deploy/helm/values-dev.yaml` contents

Safe hardcoded defaults for local testing:

```yaml
nexorious:
  secretKey: "local-dev-secret-key"
  internalApiKey: "local-dev-internal-key"
  postgresql:
    password: "nexorious-dev"

ingress:
  api:
    enabled: true
    className: nginx
    hosts:
      - host: nexorious.local
        paths:
          - path: /
            pathType: Prefix
            service:
              identifier: api
              port: http
```

### `.env.example` (root-level)

```
# IGDB API credentials — required for docker-compose and Helm dev cluster
# Get from: https://api.igdb.com/v4/getting-started
IGDB_CLIENT_ID=your-igdb-client-id
IGDB_CLIENT_SECRET=your-igdb-client-secret
```

## Notes

- `minikube start` and `minikube addons enable` are both idempotent — safe to re-run.
- The `dev` image tag corresponds to `ghcr.io/drzero42/nexorious:dev` (built on every push to main).
- To reach the app after deploy, add `$(minikube ip) nexorious.local` to `/etc/hosts`, then run `minikube tunnel` in a separate terminal.
