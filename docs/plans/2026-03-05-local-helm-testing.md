# Local Helm Testing Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Set up a local Helm chart testing workflow using minikube (KVM2 driver) with a Taskfile, covering cluster lifecycle, addon setup, and chart deployment with environment-sourced IGDB credentials.

**Architecture:** Three new files — a root-level `.env.example` documenting required env vars, a `deploy/helm/values-dev.yaml` with safe hardcoded dev defaults, and a `Taskfile.yml` at the repo root orchestrating the minikube and Helm workflow. No existing files are modified.

**Tech Stack:** [go-task (Taskfile v3)](https://taskfile.dev/), minikube (kvm2 driver), Helm 3, bash shell parsing for `.env`.

---

### Task 1: Create root `.env.example`

**Files:**
- Create: `.env.example`

The docker-compose stack already reads `IGDB_CLIENT_ID` and `IGDB_CLIENT_SECRET` from a root `.env` file, but no example file exists at the root to document this. Fix that.

**Step 1: Create the file**

```
# .env.example
# IGDB API credentials — required for docker-compose and the Helm dev cluster
# Obtain from: https://api.igdb.com/v4/getting-started
IGDB_CLIENT_ID=your-igdb-client-id
IGDB_CLIENT_SECRET=your-igdb-client-secret
```

**Step 2: Verify `.env` is gitignored**

Run: `grep -n '\.env' .gitignore`

Expected: a line matching `.env` (without the `.example` suffix). If `.env` is not gitignored, add it.

**Step 3: Commit**

```bash
git add .env.example
git commit -m "docs: add root .env.example for IGDB credentials"
```

---

### Task 2: Create `deploy/helm/values-dev.yaml`

**Files:**
- Create: `deploy/helm/values-dev.yaml`

This file contains safe hardcoded defaults for local dev (no real secrets), plus ingress configuration pointing at `nexorious.local`. It is layered on top of the main `values.yaml` via `-f` during `helm upgrade --install`.

**Step 1: Create the file**

```yaml
# deploy/helm/values-dev.yaml
# Local development overrides for the Nexorious Helm chart.
# Safe to commit — contains no real secrets.
# Used by: task dev:up / task dev:deploy
#
# To reach the app after deploy:
#   1. Add "$(minikube ip) nexorious.local" to /etc/hosts
#   2. Run "minikube tunnel" in a separate terminal

nexorious:
  secretKey: "local-dev-secret-key-not-for-production"
  internalApiKey: "local-dev-internal-key-not-for-production"
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

**Step 2: Verify the override renders correctly**

Run (from repo root — requires helm and the chart dependency to be fetched):

```bash
helm dependency update deploy/helm/
helm template nexorious deploy/helm/ \
  -f deploy/helm/values-dev.yaml \
  --set nexorious.igdbClientId=test \
  --set nexorious.igdbClientSecret=test \
  --set controllers.api.containers.main.image.tag=dev \
  --set controllers.worker.containers.main.image.tag=dev \
  --set controllers.scheduler.containers.main.image.tag=dev \
  | grep -A5 'kind: Ingress'
```

Expected: an Ingress resource with host `nexorious.local` and `ingressClassName: nginx`.

**Step 3: Commit**

```bash
git add deploy/helm/values-dev.yaml
git commit -m "helm: add values-dev.yaml for local minikube testing"
```

---

### Task 3: Create `Taskfile.yml`

**Files:**
- Create: `Taskfile.yml`

The Taskfile uses [go-task v3 syntax](https://taskfile.dev/). Key design points:

- `check-env` is `internal: true` — it is not listed in `task --list` but is called as a dependency by deploy tasks.
- IGDB credentials are extracted from `.env` using `grep` + `cut` in a `vars:` shell block — this runs in the task's shell context before the commands execute.
- `TAG` defaults to `dev` via `{{.TAG | default "dev"}}`.
- `minikube start` and `minikube addons enable` are both idempotent — safe to re-run on an existing cluster.
- `helm dependency update` is run before every deploy to ensure the `common` chart dependency is present.

**Step 1: Create the file**

```yaml
# Taskfile.yml
# Task runner for local Helm chart testing with minikube (KVM2 driver).
# Usage:
#   task dev:up            # Start cluster and deploy chart (TAG=dev)
#   task dev:up TAG=1.2.3  # Same with a specific image tag
#   task dev:deploy        # Deploy/upgrade chart only (cluster must be running)
#   task dev:down          # Uninstall release and stop minikube
#
# Prerequisites:
#   - minikube installed with kvm2 driver
#   - helm installed
#   - .env file at repo root with IGDB_CLIENT_ID and IGDB_CLIENT_SECRET

version: '3'

vars:
  TAG: '{{.TAG | default "dev"}}'
  RELEASE: nexorious
  CHART: deploy/helm

tasks:

  # ---------------------------------------------------------------------------
  # Public tasks
  # ---------------------------------------------------------------------------

  dev:up:
    desc: "Start minikube cluster, enable addons, and deploy the Helm chart (TAG={{.TAG}})"
    cmds:
      - task: minikube:start
      - task: minikube:addons
      - task: helm:deploy

  dev:deploy:
    desc: "Deploy/upgrade the Helm chart only — cluster must already be running (TAG={{.TAG}})"
    cmds:
      - task: helm:deploy

  dev:down:
    desc: "Uninstall the Helm release and stop minikube"
    cmds:
      - helm uninstall {{.RELEASE}} --ignore-not-found
      - minikube stop

  minikube:start:
    desc: "Start minikube with the KVM2 driver (idempotent)"
    cmds:
      - minikube start --driver=kvm2 --cpus=4 --memory=8192 --disk-size=20g

  minikube:addons:
    desc: "Enable storage-provisioner and ingress addons (idempotent)"
    cmds:
      - minikube addons enable storage-provisioner
      - minikube addons enable ingress

  # ---------------------------------------------------------------------------
  # Internal tasks (not shown in task --list)
  # ---------------------------------------------------------------------------

  check-env:
    internal: true
    desc: "Verify .env exists and contains IGDB credentials"
    cmds:
      - |
        if [ ! -f .env ]; then
          echo "ERROR: .env file not found."
          echo "Copy .env.example and fill in your IGDB credentials:"
          echo "  cp .env.example .env"
          exit 1
        fi
      - |
        if ! grep -q '^IGDB_CLIENT_ID=' .env; then
          echo "ERROR: IGDB_CLIENT_ID not found in .env"
          exit 1
        fi
      - |
        if ! grep -q '^IGDB_CLIENT_SECRET=' .env; then
          echo "ERROR: IGDB_CLIENT_SECRET not found in .env"
          exit 1
        fi

  helm:deploy:
    internal: true
    desc: "Run helm upgrade --install with dev values and env-sourced IGDB creds"
    deps: [check-env]
    vars:
      IGDB_CLIENT_ID:
        sh: grep '^IGDB_CLIENT_ID=' .env | cut -d= -f2-
      IGDB_CLIENT_SECRET:
        sh: grep '^IGDB_CLIENT_SECRET=' .env | cut -d= -f2-
    cmds:
      - helm dependency update {{.CHART}}
      - |
        helm upgrade --install {{.RELEASE}} {{.CHART}} \
          -f {{.CHART}}/values-dev.yaml \
          --set nexorious.igdbClientId={{.IGDB_CLIENT_ID}} \
          --set nexorious.igdbClientSecret={{.IGDB_CLIENT_SECRET}} \
          --set controllers.api.containers.main.image.tag={{.TAG}} \
          --set controllers.worker.containers.main.image.tag={{.TAG}} \
          --set controllers.scheduler.containers.main.image.tag={{.TAG}} \
          --wait --timeout=5m
```

**Step 2: Validate Taskfile syntax**

Run: `task --list`

Expected output lists `dev:up`, `dev:deploy`, `dev:down`, `minikube:start`, `minikube:addons` with their descriptions. `check-env` and `helm:deploy` should NOT appear (they are internal).

**Step 3: Dry-run the deploy task to verify variable parsing**

Ensure `.env` exists with dummy values, then run:

```bash
echo "IGDB_CLIENT_ID=test-id" > /tmp/test.env
echo "IGDB_CLIENT_SECRET=test-secret" >> /tmp/test.env
cp .env .env.bak 2>/dev/null || true
cp /tmp/test.env .env

task dev:deploy --dry  # prints commands without executing
```

Expected: the printed `helm upgrade --install` command shows `--set nexorious.igdbClientId=test-id` and `--set nexorious.igdbClientSecret=test-secret`.

Restore `.env` if you backed it up: `mv .env.bak .env 2>/dev/null || rm .env`

**Step 4: Commit**

```bash
git add Taskfile.yml
git commit -m "feat: add Taskfile for local minikube Helm chart testing"
```

---

## Post-Implementation: Manual Smoke Test

After all three tasks are complete, run a full end-to-end smoke test:

```bash
# 1. Start cluster and deploy
task dev:up

# 2. Check all pods are Running
kubectl get pods

# 3. Get minikube IP and add /etc/hosts entry
echo "$(minikube ip) nexorious.local" | sudo tee -a /etc/hosts

# 4. In a separate terminal, run tunnel
minikube tunnel

# 5. Verify ingress responds
curl -s -o /dev/null -w "%{http_code}" http://nexorious.local/health
# Expected: 200

# 6. Tear down
task dev:down
```
