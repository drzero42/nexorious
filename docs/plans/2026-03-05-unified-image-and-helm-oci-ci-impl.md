# Unified Image CI + Helm OCI Publishing Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Update the build workflow to trigger on GitHub Releases instead of tag pushes, fix the Helm chart to reference the real unified image, and publish the chart as an OCI artifact to ghcr.io on every main commit and release.

**Architecture:** All changes live in two files — `.github/workflows/build-push.yaml` (trigger + new chart job) and `deploy/helm/values.yaml` (image repo + empty tags). No new files. The chart job depends on the image job so a failed image build never produces a chart push.

**Tech Stack:** GitHub Actions, `helm` CLI, `docker/metadata-action@v5`, `oci://ghcr.io/drzero42/charts`

---

### Task 1: Create feature branch

**Step 1: Create and switch to branch**

```bash
cd /home/abo/workspace/home/nexorious
git checkout -b feat/helm-oci-publish
```

**Step 2: Verify you're on the branch**

```bash
git branch --show-current
```
Expected output: `feat/helm-oci-publish`

---

### Task 2: Fix image references in `values.yaml`

**Files:**
- Modify: `deploy/helm/values.yaml` (lines 188-190, 236-238, 271-273)

There are three controllers that share the same image. Update each one.

**Step 1: Update `controllers.api` image**

In `deploy/helm/values.yaml`, find the `api` controller block (around line 188):

```yaml
      main:
        image:
          # -- API container image repository. Change to your actual image.
          repository: ghcr.io/your-org/nexorious-api
          tag: latest
          pullPolicy: IfNotPresent
```

Change to:

```yaml
      main:
        image:
          repository: ghcr.io/drzero42/nexorious
          tag: ""
          pullPolicy: IfNotPresent
```

(Remove the comment — it's no longer needed.)

**Step 2: Update `controllers.worker` image**

Find the `worker` controller block (around line 236):

```yaml
      main:
        image:
          # -- Same image as the api controller
          repository: ghcr.io/your-org/nexorious-api
          tag: latest
          pullPolicy: IfNotPresent
```

Change to:

```yaml
      main:
        image:
          repository: ghcr.io/drzero42/nexorious
          tag: ""
          pullPolicy: IfNotPresent
```

**Step 3: Update `controllers.scheduler` image**

Find the `scheduler` controller block (around line 271):

```yaml
      main:
        image:
          # -- Same image as the api controller
          repository: ghcr.io/your-org/nexorious-api
          tag: latest
          pullPolicy: IfNotPresent
```

Change to:

```yaml
      main:
        image:
          repository: ghcr.io/drzero42/nexorious
          tag: ""
          pullPolicy: IfNotPresent
```

**Step 4: Verify no old references remain**

```bash
grep -n "your-org\|nexorious-api" deploy/helm/values.yaml
```
Expected: no output.

**Step 5: Lint the chart**

```bash
cd /home/abo/workspace/home/nexorious
helm lint --strict deploy/helm/ \
  --set nexorious.secretKey=x \
  --set nexorious.internalApiKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x
```
Expected: `1 chart(s) linted, 0 chart(s) failed`

**Step 6: Commit**

```bash
git add deploy/helm/values.yaml
git commit -m "fix(helm): update image references to unified ghcr.io/drzero42/nexorious"
```

---

### Task 3: Update workflow trigger

**Files:**
- Modify: `.github/workflows/build-push.yaml` (lines 4-7)

**Step 1: Replace the `on:` block**

Current:
```yaml
on:
  push:
    branches: [main]
    tags: ["v*"]
```

Replace with:
```yaml
on:
  push:
    branches: [main]
  release:
    types: [published]
```

**Step 2: Validate workflow YAML syntax**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/build-push.yaml'))" && echo "YAML OK"
```
Expected: `YAML OK`

**Step 3: Commit**

```bash
git add .github/workflows/build-push.yaml
git commit -m "ci: trigger on GitHub release published instead of tag push"
```

---

### Task 4: Add `build-push-chart` job to workflow

**Files:**
- Modify: `.github/workflows/build-push.yaml` (append after `build-push` job)

**Step 1: Append the new job**

Add the following at the end of `.github/workflows/build-push.yaml`:

```yaml
  build-push-chart:
    name: Build and Push Helm Chart
    runs-on: ubuntu-latest
    needs: build-push

    permissions:
      contents: read
      packages: write

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Compute chart version
        id: chart-version
        run: |
          if [[ "${{ github.event_name }}" == "release" ]]; then
            VERSION="${{ github.event.release.tag_name }}"
            VERSION="${VERSION#v}"
            APP_VERSION="$VERSION"
          else
            SHORT_SHA=$(git rev-parse --short HEAD)
            DATE=$(date +%Y%m%d)
            VERSION="0.0.0-dev-${DATE}-${SHORT_SHA}"
            APP_VERSION="dev"
          fi
          echo "version=$VERSION" >> $GITHUB_OUTPUT
          echo "app_version=$APP_VERSION" >> $GITHUB_OUTPUT

      - name: Build Helm dependencies
        run: helm dependency update deploy/helm/

      - name: Package Helm chart
        run: |
          helm package deploy/helm/ \
            --version ${{ steps.chart-version.outputs.version }} \
            --app-version ${{ steps.chart-version.outputs.app_version }}

      - name: Log in to ghcr.io (Helm)
        run: |
          echo "${{ secrets.GITHUB_TOKEN }}" | \
            helm registry login ghcr.io \
              --username ${{ github.actor }} \
              --password-stdin

      - name: Push Helm chart
        run: helm push nexorious-*.tgz oci://ghcr.io/drzero42/charts
```

**Step 2: Validate YAML syntax**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/build-push.yaml'))" && echo "YAML OK"
```
Expected: `YAML OK`

**Step 3: Spot-check the full workflow structure**

```bash
python3 -c "
import yaml
wf = yaml.safe_load(open('.github/workflows/build-push.yaml'))
print('Triggers:', list(wf['on'].keys()))
print('Jobs:', list(wf['jobs'].keys()))
print('Chart job needs:', wf['jobs']['build-push-chart']['needs'])
"
```
Expected output:
```
Triggers: ['push', 'release']
Jobs: ['test', 'build-push', 'build-push-chart']
Chart job needs: build-push
```

**Step 4: Commit**

```bash
git add .github/workflows/build-push.yaml
git commit -m "ci: build and push Helm chart as OCI artifact to ghcr.io/drzero42/charts"
```

---

### Task 5: Open PR

**Step 1: Push branch**

```bash
git push -u origin feat/helm-oci-publish
```

**Step 2: Open PR**

```bash
gh pr create \
  --title "ci: unified image CI and Helm OCI publishing" \
  --body "$(cat <<'EOF'
## Summary

- Switch workflow trigger from tag push to GitHub Release published
- Fix Helm chart image references: `ghcr.io/your-org/nexorious-api` → `ghcr.io/drzero42/nexorious`, `tag: latest` → `tag: ""` (falls back to `.Chart.AppVersion`)
- Add `build-push-chart` job that packages and pushes the Helm chart as an OCI artifact to `oci://ghcr.io/drzero42/charts` on every main commit (dev version) and every release (semver version)

## Test plan

- [ ] Verify `helm lint` passes locally after values.yaml change
- [ ] Verify YAML syntax of updated workflow is valid
- [ ] After merge, confirm a main-branch build produces a chart push at `ghcr.io/drzero42/charts/nexorious:0.0.0-dev-*`
- [ ] On next release, confirm chart is published as `ghcr.io/drzero42/charts/nexorious:X.Y.Z`
- [ ] Confirm chart can be installed: `helm install nexorious oci://ghcr.io/drzero42/charts/nexorious --version X.Y.Z`

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Notes

- The `helm dependency update` step fetches `charts/common-4.6.2.tgz` from bjw-s. If the tgz is already committed in `deploy/helm/charts/`, this is a no-op but harmless.
- `helm push` requires Helm 3.8+. GitHub-hosted runners ship with Helm 3.x by default — no setup action needed.
- The `build-push-chart` job inherits the same `permissions` block as `build-push` (`packages: write`) — required to push OCI artifacts to ghcr.io.
- Chart installable after release: `helm install nexorious oci://ghcr.io/drzero42/charts/nexorious --version 1.2.3`
