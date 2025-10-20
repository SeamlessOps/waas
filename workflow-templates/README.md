# ondgo-products Workflows

This repository contains reusable **GitHub composite actions** for secure authentication, language-specific builds, container image publishing, and GitOps integration. These workflows are designed to streamline CI/CD pipelines for .NET, Nx, and future languages in the ondgo-products ecosystem.

---

## Overview

The workflows are modular and composable, enabling consistent, secure, and maintainable pipelines across multiple services and environments:

### Authentication Workflows (`shared/auth/`)

* Authenticate against Vault to retrieve secrets securely.
* Set Vault secrets as environment variables for downstream steps.
* Authenticate with Google Cloud using Vault-provided credentials.
* Ensure all secret management is centralized and secure.

### Language-Specific Build Templates (`workload/templates/dotnet/`, `workload/templates/nx/`)

* Build and test projects in their respective language ecosystems.
* Accept inputs like service name, environment, and language-specific parameters.
* Designed to be extended easily with new language templates following the same pattern.

## General-Purpose Build Template (`workload/templates/general-purpose/`)

The **General-Purpose Build Template** is a reusable composite action designed to simplify deployments of services that do not require language-specific build steps or for projects that want a more generic pipeline.

### Build and Publish Workflow (`workload/shared/build-and-publish/`)

* Build Docker images for services, optionally leveraging language-specific build arguments (e.g., Nx project name).
* Push images to Google Artifact Registry.
* Clone and update the GitOps repository with the new image references.
* Trigger ArgoCD deployments by updating environment-specific Helm values.

---
### Features

* Authenticates with Vault to retrieve secrets securely.
* Delegates Docker image build and publish tasks to the shared `build-and-publish` workflow.
* Supports flexible inputs for service name, Dockerfile path, environment, and Vault credentials.
* Can optionally handle Nx project builds if `nx_project` input is provided.
* Designed to be easily extended or customized without writing language-specific build logic.

### Inputs

| Input               | Description                                  | Required | Default |
| ------------------- | -------------------------------------------- | -------- | ------- |
| `service`           | Service name                                 | Yes      | —       |
| `dockerfile`        | Path to the Dockerfile                       | Yes      | —       |
| `environment`       | Deployment environment (dev, staging, prod)  | Yes      | —       |
| `vault_addr`        | Vault server address                         | Yes      | —       |
| `vault_role`        | Vault authentication role                    | Yes      | —       |
| `vault_secret_path` | Path in Vault to fetch secrets               | Yes      | —       |
| `nx_project`        | (Optional) Nx project name to pass build arg | No       | —       |

---

### Relationship to Language-Specific Templates

While this general-purpose template covers many use cases, you can still opt for language-specific templates (such as `.NET` or `Nx`) when you need specialized build steps, testing, or tooling.

---
## Workflow Execution Flow

> [Trigger → Vault Auth → Build → GCP Auth → Docker → GitOps → ArgoCD](./cicd-flow.png)

1. **Authenticate** with Vault to retrieve secrets and expose them as environment variables.
2. **Authenticate with Google Cloud** using Vault secrets.
3. **Build the application** using language-specific composite actions.
4. **Build and push Docker images** to Artifact Registry.
5. **Update GitOps repository** with the new image tag.
6. **ArgoCD** picks up the change and deploys the updated application.

This ensures flexibility and consistency across your CI/CD workflows.

---

## Secrets Management

* Vault secrets are injected securely and passed explicitly between composite actions.
* GitHub secrets like `VAULT_ADDR`, `VAULT_ROLE`, and `VAULT_SECRET_PATH` must be configured in repository secrets as handled by Terraform.
* The Git token used to update the GitOps repository (`git_token`) should have minimum required scopes.

---

## Extensibility & Contributions

* Future language-specific build templates should follow the existing structure:

  * Receive authentication and environment inputs.
  * Build and test projects accordingly.
  * Integrate seamlessly with the `build-and-publish` workflow.
* See [`CONTRIBUTING.md`](./CONTRIBUTING.md) for guidelines on adding new language templates, naming conventions, security practices, and testing requirements.

---

## Updating Documentation for New Language Templates

As new language-specific build templates are introduced, please ensure the README is updated accordingly to keep information accurate and comprehensive. Specifically:

* Add the new language template folder and description under **Language-Specific Build Templates**.
* Update the **Overview** and **Extensibility & Contributions** sections to reference the new template.
* Provide any relevant input parameters, usage notes, or special instructions.
* Include example usage snippets if applicable.

Keeping the documentation up to date helps all contributors and users understand the evolving workflow capabilities and fosters consistent adoption of new templates.

---

## Example Usage

Here is a simplified example of how these workflows might be chained in a repository workflow:

```yaml
jobs:
  deploy-staging:
    name: Deploy identity service to staging
    runs-on: ubuntu-latest
    environment: staging
    permissions:
      id-token: write
      contents: read

    steps:
      - name: Checkout repository
        uses: actions/checkout@v4

      - name: Deploy to staging
        uses: SeamlessOps/waas/workflow-templates/workload/templates/general-purpose@main
        with:
          service: identity-ondgo-ng
          environment: staging
          dockerfile: ondgo.identity/Dockerfile.dev
          vault_addr: ${{ secrets.VAULT_ADDR }}
          vault_role: ${{ secrets.VAULT_ROLE }}
          vault_secret_path: ${{ secrets.VAULT_SECRET_PATH }}
```

---

## Support

For questions, bugs, or feature requests, please open an issue or join the team discussions.
