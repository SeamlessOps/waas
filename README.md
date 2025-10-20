
# Ondgo Products
Central Helm chart repository for deploying Ondgo products across all environments. This chart manages the deployment of all Ondgo services with environment-specific configurations.

![Helmit](https://img.shields.io/badge/Helmit-Helm%20Chart-blue)
![ArgoCD](https://img.shields.io/badge/Deployed%20with-ArgoCD-green)
![ArgoCD](https://img.shields.io/badge/Rollout%20with-ArgoRollout-green)

## 📂 Repository Structure

```
ondgo-products/
├── environment/           # Environment-specific values
│   ├── development/       # Development configurations
│   │   └── values-development.yaml
│   ├── staging/           # Staging configurations
│   │   └── values-staging.yaml
│   └── production/        # Production configurations
│       └── values-production.yaml
```

## 🚀 Deployment Flow

1. **Image Updates**: Services are built and pushed to GCP Artifact Registry via GitHub Actions (GHA)
2. **Chart Updates**: Image tags are updated in the respective environment values files
3. **ArgoCD Sync**: Changes are automatically synced to Kubernetes clusters via ArgoCD
4. **Argo Rollouts**: Handles production deployments using BlueGreen or Canary strategy

## ⚙️ Configuration

### Core Values (`values-example.yaml`)

Contains default configurations for all services as adapted from [here](https://github.com/onDgoHQ/ondgo-helm-charts/blob/main/ondgo-products/values.yaml).

> **Note:** _This is only an example of possible scenarios. All services in it are disabled._

Environment-specific overrides are in respective values files.

Key configurations:
- Workload type ('deployment', 'rollout', 'statefulset')
- Resource requests/limits
- Health checks and probes
- TLS/SSL settings
- Autoscaling parameters
- Ingress configurations
To learn more about the default configuration, click [here]([link](https://github.com/onDgoHQ/ondgo-helm-charts/blob/main/ondgo-products/values.yaml)).

### Service Configuration Example

```yaml
services:
  # Frontend Service Example
  account-ondgo-dev:
    tag: image-tag
  
  # Backend Service Example
  api-ondgo-dev:
    tag: image-tag
    frontend: false          # Marks as backend service
    autoscale:               # Enables horizontal pod autoscaling
      enabled: true
      maxReplicas: 2           # Maximum pods when scaling
    resources:
      requests:
        cpu: "50m"
        memory: "250Mi"
      limits:
        cpu: "100m"
        memory: "500Mi"
    healthcheckPath: /health  # Custom health check path
```

## 🌍 Environment Management

Each environment has its own values file that overrides the defaults:

| Environment | Values File | Purpose |
|-------------|------------|---------|
| Development | [`environment/development/values-development.yaml`](environment/development/values-development.yaml) | Local testing and developer environments |
| Staging | [`environment/staging/values-staging.yaml`](environment/staging/values-staging.yaml) | Pre-production testing |
| Production | [`environment/production/values-production.yaml`](environment/production/values-production.yaml) | Live customer-facing environment |

## 🛠️ Common Operations

### Update a Service Image
1. Update the image tag in the appropriate environment values file
2. Commit and push changes
3. ArgoCD will automatically sync the changes
4. Argo Rollout will now handle production deployments using BlueGreen or Canary strategy.

### Add a New Service
1. Add service configuration to the appropriate environment values file
2. Ensure required parameters are set:
   - `image` (required)
   - `workload` (deployment, rollout, statefulset -> `deployment` by default)
   - `frontend` (true/false)
   - Resource requests/limits
   - Health check path

### Modify Autoscaling
```yaml
services:
  service-ondgo-dev:
    autoscale: 
      enabled: true
      maxReplicas: 4
      cpuUtilization: 70  # Target CPU% for scaling
```

```yaml
services:
  service-ondgo-ng:
    autoscale: 
      enabled: true
      maxReplicas: 5
      cpuUtilization: 80  # Target CPU% for scaling
```

## 🔒 Security

- TLS is configurable per-environment
- Certificates are managed via cert-manager
- Resource limits prevent runaway consumption

## 🌐 URL Routing Convention

### Domain Structure
| Environment | Domain Suffix | Service Name Pattern       | Example URL              |
|-------------|---------------|----------------------------|--------------------------|
| Staging     | `.ondgo.dev`  | `service-ondgo-dev`        | `api-ondgo-dev` → `api.ondgo.dev` |
| Production  | `.ondgo.ng`   | `service-ondgo-ng`         | `payment-ondgo-ng` → `payment.ondgo.ng` |

### URL Generation Rules
1. **Automatic Conversion**:
   ```text
   service-ondgo-dev  →  service.ondgo.dev
   service-ondgo-ng   →  service.ondgo.ng
   ```
   - The `-ondgo-<env>` suffix is automatically removed
   - The correct domain suffix is applied based on environment

2. **Custom Overrides**:
   ```yaml
   services:
     special-ondgo-ng:
       host: "premium.ondgo.ng"  # Full custom domain
   ```

2. **Multiple Hosts:**
You can specify multiple hosts to point to one service
   ```yaml
   services:
      airline-ondgo-ng:
         host:
            - "airline.ondgo.ng"
            - "client.airline.ondgo.ng"
            - "client.url.com"
   ```

3. **Multi BlueGreen rollout example with 'active' and 'preview' hosts**

   ```yaml
    services:
      multi-host-blue-green-rollout-example:
        tag: image-tag
        workload: rollout
        rollout:
          strategy:
            blueGreen:
              enabled: true
        hosts:
          active: green.ondgo.ng
          preview: blue.ondgo.ng

4. **Global Rollout Configuration**

    ```yaml
      # Global values for all services
      global:
        workload: rollout
      rollout:
        strategy:
          blueGreen:
            enabled: true
    ```

### Required Service Naming
```yaml
services:
  # Staging Example
  api-ondgo-dev:
    tag: api-ondgo-dev-image-tag"
    # Exposed at: api.ondgo.dev

  # Production Example 
  payments-ondgo-ng:
    tag: payments-ondgo-ng-image-tag"
    # Exposed at: payments.ondgo.ng

  # Custom Domain Example
  admin-ondgo-ng:
    host: "console.ondgo.ng"  # Override
```

### Enforcement Rules
1. All production services must use `-ondgo-ng` suffix
2. All staging services must use `-ondgo-dev` suffix  
3. Never mix environments in service names
4. Custom domains must:
   - Use approved base domains (.ondgo.ng or .ondgo.dev)
   - Be registered in DNS
   - Follow company naming standards

## 🛠️ Troubleshooting URL Issues
1. Verify service name matches pattern:
   ```sh
   # Check naming convention
   kubectl get ingress -n ondgo-(dev/ng) --show-labels
   ```
2. Confirm DNS resolution:
   ```sh
   dig +short service.ondgo.ng
   ```
3. Check ingress controller logs:
   ```sh
   kubectl logs -n ondgo-ingress-nginx deploy/ondgo-core-ingress-nginx-controller-(production/staging)
   ```

## ❓ Support

For issues with deployments:
1. Check ArgoCD [sync status](https://argocd.ondgo.ng)
2. Check Argo Rollouts for [rollout status](https://rollouts.ondgo.ng)
3. Verify image tags in the values files
4. Confirm Kubernetes cluster has necessary resources

---
**Maintained by** Ondgo Platform Team | Email(go@ondgo.ng)
