# Helm Charts

This directory contains Helm charts for the Fleet Management Operator.

## Publishing to GitHub Pages

To make your Helm chart available via `helm repo add`, you can host it on GitHub Pages.

### Setup

1. **Package the chart:**
   ```bash
   cd charts
   helm package fleet-management-operator
   ```
   This creates `fleet-management-operator-0.1.0.tgz`

2. **Create/update the index:**
   ```bash
   helm repo index . --url https://YOUR_USERNAME.github.io/fm-crd/charts
   ```
   This creates/updates `index.yaml`

3. **Commit and push:**
   ```bash
   git add fleet-management-operator-*.tgz index.yaml
   git commit -m "Release Helm chart v0.1.0"
   git push
   ```

4. **Enable GitHub Pages:**
   - Go to your repo Settings → Pages
   - Source: Deploy from a branch
   - Branch: main, folder: /charts
   - Save

### Using the Helm Repository

After GitHub Pages is enabled, users can install with:

```bash
helm repo add fm-operator https://YOUR_USERNAME.github.io/fm-crd/charts
helm repo update
helm install fleet-management-operator fm-operator/fleet-management-operator \
  --namespace fleet-management-system \
  --create-namespace \
  --set fleetManagement.baseUrl='https://fleet-management-prod-011.grafana.net/pipeline.v1.PipelineService/' \
  --set fleetManagement.username='YOUR_STACK_ID' \
  --set fleetManagement.password='YOUR_TOKEN'
```

## GitHub Actions Automation (Optional)

Create `.github/workflows/release-chart.yaml`:

```yaml
name: Release Helm Chart

on:
  push:
    tags:
      - 'chart-v*'

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Configure Git
        run: |
          git config user.name "$GITHUB_ACTOR"
          git config user.email "$GITHUB_ACTOR@users.noreply.github.com"

      - name: Install Helm
        uses: azure/setup-helm@v3

      - name: Package chart
        run: |
          cd charts
          helm package fleet-management-operator

      - name: Update chart index
        run: |
          cd charts
          helm repo index . --url https://${{ github.repository_owner }}.github.io/fm-crd/charts

      - name: Commit and push
        run: |
          git add charts/*.tgz charts/index.yaml
          git commit -m "Release chart ${{ github.ref_name }}"
          git push
```

Then release with:
```bash
git tag chart-v0.1.0
git push origin chart-v0.1.0
```

## Local Development

Install from local chart:

```bash
cd charts/fleet-management-operator
helm install fleet-management-operator . \
  --namespace fleet-management-system \
  --create-namespace \
  -f values-example.yaml
```

Test rendering:

```bash
helm template test . \
  --set fleetManagement.baseUrl=https://test \
  --set fleetManagement.username=test \
  --set fleetManagement.password=test
```

Lint the chart:

```bash
helm lint . \
  --set fleetManagement.baseUrl=https://test \
  --set fleetManagement.username=test \
  --set fleetManagement.password=test
```
