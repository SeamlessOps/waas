# WaaS

WaaS (Workflow as a Service) is a set of GitHub Actions workflows and composite actions that build services and deploy them through a GitOps repository. A service repository calls one reusable workflow; WaaS works out which services to deploy and to which environment, builds and publishes their images, and writes the new image tags into the GitOps repository, where a GitOps controller such as Argo CD picks them up.

It also has a workflow for Terraform: lint, security scans, plan and apply.

## How a deployment runs

[WAAS.md](.github/workflows/docs/WAAS.md) shows the architecture and the flow in diagrams.

The workload workflow, [`.github/workflows/waas.yml`](.github/workflows/waas.yml), runs on every push from a merged pull request; the branch decides the environment:

1. **Load services.** [`load-services-matrix`](workflow-templates/workload/utils/load-services-matrix) reads the services file and decides what the repository deploys: one service (a polyrepo) or several (a monorepo), and which of them deploy to each environment.
2. **Route.** [`router`](workflow-templates/workload/waas/router) finds the environment the branch deploys to, from the environments the services file declares (by default `dev` to staging and `main` to production). It gives the environment's name, its domain and the build configuration, and the services that deploy there. A branch no environment takes deploys nothing.
3. **Deploy each service**, in parallel, with [`deployer`](workflow-templates/workload/waas/deployer):
   - reads its credentials from Vault;
   - skips the service when its values already hold the tag this run would write, as on a re-run;
   - builds the project with the template for its project type, for `dotnet` and `nx`;
   - builds and publishes the Docker image;
   - writes the image tag to the service's values file in the GitOps repository;
   - sends the deployment's metrics to the WaaS dashboard API.
4. **Notify.** Posts the result to Slack when something was deployed.

On a pull request, the workflow runs the pull request checks only.

## Use it

Add a workflow to the service repository:

```yaml
name: Deploy

on:
  pull_request:
  push:
    branches: [dev, main]   # the branches your environments take

jobs:
  deploy:
    uses: rollwright/waas/.github/workflows/waas.yml@main
    with:
      project_type: dotnet
    permissions:
      id-token: write
      contents: read
    secrets: inherit
```

The repository needs the secrets `VAULT_ADDR`, `VAULT_ROLE` and `VAULT_SECRET_PATH`: Vault holds the tokens for the GitOps repository and the registry.

An organization can also keep its own copy of `waas.yml` in its GitOps repository, with its own defaults, and have its service repositories call that. The copy uses WaaS's actions as they are.

WaaS has no releases yet; workflows and actions are referenced at `@main`.

### Inputs

| Input | Default | Description |
|---|---|---|
| `project_type` | required | `dotnet` (or `.net`) and `nx` build with their template before the image; `generic`, `node`, `python` and `java` build the image only. A service in the services file can set its own |
| `monorepo` | `true` | The repository deploys several services |
| `service` | the repository's name | The service a polyrepo deploys as |
| `services_dir` | `.github/workflows/services` | Directory of the services files, in the GitOps repository |
| `services_file` | `<repository>.json` | The repository's services file |
| `load_default` | `true` | Read the services file from the GitOps repository; `false` reads it from the calling repository |
| `dockerfile` | `Dockerfile` | Dockerfile to build |
| `dotnet_version` | `9.0.x` | .NET SDK version |
| `node_version` | `22.2.x` | Node.js version |
| `staging_env`, `production_env` | `staging`, `production` | Environment names, when the services file declares no environments |
| `staging_branch`, `production_branch` | `dev`, `main` | The branch that deploys to each, likewise |
| `staging_domain`, `production_domain` | `staging`, `production` | Domain of each, likewise; a service's values key is `<name>-<domain>` |
| `staging_config`, `production_config` | `Release`, `Release` | Build configuration of each, likewise |
| `dashboard_url` | none | Link to a deployment in the GitOps controller, `{environment}` replaced; shown in the run's summary |
| `dry-run` | `false` | Load and route without deploying |

## The services file

A services file lists what a repository deploys. It lives in the GitOps repository, in `services_dir`, either as the repository's own file (`<repository>.json`) or as entries in a shared catalog (`services.json`) that many repositories use.

```json
{
  "metadata": {
    "default_environments": ["staging", "production"]
  },
  "repositories": {
    "shop": { "project_type": "nx", "version": { "node": "24.x" } }
  },
  "services": [
    { "repository": "shop", "name": "web", "directory": "apps/web" },
    { "repository": "shop", "name": "admin", "directory": "apps/admin", "environments": ["staging"] }
  ]
}
```

- `name` is what the service deploys as: its image and, with the environment's domain, its key in the GitOps values.
- `environments` lists where it deploys. Without it, an entry deploys to `metadata.default_environments`, or to every environment.
- `repositories` holds a repository's defaults. Each of its entries is laid over them: the entry's own fields win, and objects such as `version` merge key by key.
- Build options can be set per entry or per repository: `project_type`, `service_type`, `version` (`dotnet`, `node`), `directory`, `nx_project`, `dockerfile`, `docker_target`, `promote`. Fields WaaS does not read are passed through to the deployment.

### Environments and branches

`metadata.environments` declares where services deploy, and which branch deploys to each:

```json
"metadata": {
  "environments": [
    { "name": "staging",    "branch": "dev",  "domain": "shop-staging", "build_configuration": "Release" },
    { "name": "production", "branch": "main", "domain": "shop",         "build_configuration": "Release" }
  ]
}
```

- A push to an environment's branch deploys there; a push to any other branch deploys nothing.
- `domain` completes each service's values key, `<name>-<domain>`; `build_configuration` is passed to the build.
- The order is the promotion path: an environment can promote the image the one before it built.
- A repository whose branches differ maps its own in `repositories.<repository>.branches`, an object of branch to environment: `{ "develop": "staging", "main": "production" }`. It replaces the file's branches for that repository; an environment it doesn't map gets nothing from it.

Without `metadata.environments`, the workflow's `staging_*` and `production_*` inputs give two environments. [`clients/example`](clients/example) has a complete file.

### Repositories

A repository without a file of its own is looked up in the catalog by its name. One entry deploys as a polyrepo under the entry's `name`; several deploy as a monorepo.

## The GitOps repository

Each environment's values are in `environment/<environment>/`, as YAML files with a `services` map. A service's image tag is `services.<name>-<domain>.tag`, in exactly one file of its environment. WaaS changes only that field.

## Promotion

The deployer can promote instead of building: when production deploys code that staging already built, it copies staging's image to the production name and writes production's tag. It does so when the comparison between the commit staging's tag was built from and the commit being deployed lists no changed files; otherwise it builds. The router gives the environment to promote from (`promote_from_environment`, `promote_from_domain`: the one before in `metadata.environments`); the calling workflow enables promotion by passing them to the deployer as `promote_from_environment` and `promote_from_service` (`<name>-<domain>`). A service opts out with `"promote": false`. Images must take their settings at runtime for this to be safe.

## Metrics and the dashboard

Every deployment is recorded on the WaaS dashboard: its environment, service, status, duration, commit, image tag and workflow run. `collect-metrics` sends the record to `POST <waas_api_url>/api/deployments` with a workflow API key. The dashboard shows the history and its statistics by environment, service, status and time.

The dashboard's source is [`waas-dashboard`](https://github.com/SeamlessOps/project_name-products/tree/feature/decoupling/waas-dashboard), on the `feature/decoupling` branch of `SeamlessOps/project_name-products`.

## Infrastructure

[`.github/workflows/iac-workflow-as-a-service.yml`](.github/workflows/iac-workflow-as-a-service.yml) runs Terraform for a working directory:

- On a pull request: TFLint and tfsec, then `terraform plan`, the plan commented on the pull request, and a Checkov scan of the plan. Failed scans are posted to Slack, and a failed plan gets an AI explanation.
- After merge: `terraform apply`, with the result posted to Slack.

## The `waas` CLI

The work on data (reading the services file and deciding what a repository deploys) is a Go program in [`cmd/waas`](cmd/waas), not shell. An action builds it from its own checkout of WaaS, so the CLI always matches the action. It runs the same locally:

```bash
go run ./cmd/waas validate -f services.json
go run ./cmd/waas matrix -f services/shop.json --catalog services/services.json --repository shop
```

`validate` checks a services file and reports every problem. `matrix` prints what a repository deploys, per environment, as JSON; inside GitHub Actions it also sets the step's outputs (`monorepo`, `service`, `all-services`, `<environment>-services`) and summary.

## Repository layout

| Path | Contents |
|---|---|
| `.github/workflows/` | The reusable workflows: `waas.yml` (workloads), `iac-workflow-as-a-service.yml` (Terraform); `docs/WAAS.md`, the architecture |
| `workflow-templates/workload/` | Workload actions: loading services, routing, deploying, project templates, GitOps updates, metrics |
| `workflow-templates/infrastructure/` | Terraform actions: setup, plan, apply, security scans |
| `workflow-templates/shared/` | Vault and gcloud authentication, Slack, pull request checks |
| `cmd/`, `internal/` | The `waas` CLI |
| `clients/` | Example caller workflows and services files |

## Development

`go test ./...` runs the CLI's tests; the `Go` workflow runs them, with `gofmt` and `go vet`, on every pull request that changes Go code.

To add a project type, add its build steps as an action in `workflow-templates/workload/templates/<project_type>/action.yml` and call it from the deployer for that `project_type`.
