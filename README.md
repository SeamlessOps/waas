# WaaS

WaaS (Workflow as a Service) is a set of GitHub Actions workflows and composite actions that build services and deploy them through a GitOps repository. A service repository calls one reusable workflow; WaaS works out which services to deploy and to which environment, builds and publishes their images, and writes the new image tags into the GitOps repository, where a GitOps controller such as Argo CD picks them up.

It also has a workflow for Terraform: lint, security scans, plan and apply.

## How a deployment runs

[WAAS.md](.github/workflows/docs/WAAS.md) shows the architecture and the flow in diagrams.

The workload workflow, [`.github/workflows/waas.yml`](.github/workflows/waas.yml), runs in four stages; a push from a merged pull request deploys, and its branch decides the environment:

1. **Validate.** On a pull request, the workflow runs the pull request checks and stops there.
2. **Plan.** [`plan`](workflow-templates/workload/waas/plan) runs `waas plan`: it reads the services file, finds the environment the branch deploys to (from the file's `metadata.environments`, by default `dev` to staging and `main` to production), and gives that environment's name, domain and build configuration and the services that deploy there. A branch no environment takes deploys nothing.
3. **Deploy** each service, in parallel, with [`deploy`](workflow-templates/workload/waas/deploy):
   - reads its credentials from Vault;
   - skips the service when its values already hold the tag this run would write, as on a re-run;
   - builds the project with the template for its project type, for `dotnet` and `nx`;
   - builds and publishes the Docker image, and signs it when the services file asks;
   - writes the image tag to the service's values file in the GitOps repository, only once the image is signed;
   - sends the deployment's metrics to the WaaS dashboard API.
4. **Notify.** Posts the result to Slack when something was deployed.

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
| `dashboard_url` | none | Link to a deployment in the GitOps controller, shown in the run's summary. `{environment}` and `{app}` are filled in; `{app}` is the values file the service is in (`environment/<environment>/<app>.yaml`), so one Argo CD application per file links as `https://argocd.example.com/applications/argocd/apps-{app}-{environment}` |
| `dry-run` | `false` | Plan without deploying |

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

Without `metadata.environments`, or without a services file, there are two: staging on `dev` and production on `main`, each named as its domain and built as `Release`. [`clients/example`](clients/example) has a complete file.

### Repositories

A repository without a file of its own is looked up in the catalog by its name. One entry deploys as a polyrepo under the entry's `name`; several deploy as a monorepo.

### Image signing

`metadata.signing` signs every image WaaS builds or promotes, with [cosign](https://github.com/sigstore/cosign), by digest:

```json
"metadata": { "signing": { "keyless": { "enabled": true } } }
```

- **Keyless** signs with the deploy workflow's GitHub identity; no key to keep. `enabled` alone is the common case: a signature is trusted when a GitHub Actions workflow made it in the same repository. `identity` (a regular expression of the signing workflows) and `issuer` narrow it. Each signature is recorded in Sigstore's public transparency log, with the image digest and the repository and workflow names; for private repositories, prefer a key.
- **A key** (`{ "key": "gcpkms://projects/<p>/locations/<l>/keyRings/<r>/cryptoKeys/<k>" }`, or any cosign key reference) keeps signatures in the registry, with nothing in a public log. The deploy job's credentials need to sign with it.

Without `metadata.signing`, nothing is signed. Keyless signing needs `id-token: write` in the calling workflow, as in the example under [Use it](#use-it).

How the chain holds:

1. **Build and sign.** An image is signed before its tag is written to the GitOps repository: an image that can't be signed never lands.
2. **Promotion verifies before it lands.** A promotion checks the source image's signature before copying it, and signs the copy. An image without a valid signature, such as one built before signing was turned on, is not promoted: it is built again, and signed.
3. **The cluster verifies what it runs.** From the GitOps repository to the cluster, any admission controller that reads cosign signatures can refuse unsigned images: Kyverno's `verifyImages`, Sigstore's policy-controller, or another of your choice, with the same key or identity.

## The GitOps repository

Each environment's values are in `environment/<environment>/`, as YAML files with a `services` map. A service's image tag is `services.<name>-<domain>.tag`, in exactly one file of its environment. WaaS changes only that field.

## Promotion

Deploy can promote instead of building: when production deploys code that staging already built, it copies staging's image to the production name and writes production's tag. It does so when the comparison between the commit staging's tag was built from and the commit being deployed lists no changed files; otherwise it builds. The plan gives the environment to promote from (`promote_from_environment`, `promote_from_domain`: the one before in `metadata.environments`); the calling workflow enables promotion by passing them to `deploy` as `promote_from_environment` and `promote_from_service` (`<name>-<domain>`). A service opts out with `"promote": false`. Images must take their settings at runtime for this to be safe. With [image signing](#image-signing), only an image with a valid signature is promoted.

## Metrics and the dashboard

Every deployment is recorded on the WaaS dashboard: its environment, service, status, duration, commit, image tag and workflow run. `collect-metrics` sends the record to `POST <waas_api_url>/api/deployments` with a workflow API key. The dashboard shows the history and its statistics by environment, service, status and time.

The dashboard's source is [`waas-dashboard`](https://github.com/SeamlessOps/project_name-products/tree/feature/decoupling/waas-dashboard), on the `feature/decoupling` branch of `SeamlessOps/project_name-products`.

## Infrastructure

[`.github/workflows/iac-workflow-as-a-service.yml`](.github/workflows/iac-workflow-as-a-service.yml) runs Terraform for a working directory:

- On a pull request: TFLint and tfsec, then `terraform plan`, the plan commented on the pull request, and a Checkov scan of the plan. Failed scans are posted to Slack, and a failed plan gets an AI explanation.
- After merge: `terraform apply`, with the result posted to Slack.

## The `waas` CLI

The work on data (reading the services file, deciding what a repository deploys, routing a branch) is a Go program in [`cmd/waas`](cmd/waas), not shell:

```bash
waas validate -f services.json
waas matrix -f services/shop.json --catalog services/services.json --repository shop
waas plan -f services/shop.json --catalog services/services.json --repository shop --branch dev
```

- `validate` checks a services file and reports every problem.
- `plan` is what a push deploys: the environment its branch takes (name, domain, build configuration), the services there, and the environment it can promote from. Everything comes from the services file; the flags only say where it is, which repository asks, and the branch (`$GITHUB_REF_NAME` by default).
- `matrix` prints what a repository deploys in every environment.

Inside GitHub Actions, each also sets the step's outputs and summary.

Each release publishes the CLI's binaries for Linux and macOS (amd64, arm64) with signed checksums. The actions get it through [`setup-waas`](workflow-templates/shared/setup-waas), which downloads the binary for the runner, verifies the checksums' signature (made by this repository's release workflow, at a version tag) and the binary, and puts `waas` on `PATH`; `version` pins a release, and the default is the latest. To use it in your own workflow:

```yaml
- uses: rollwright/waas/workflow-templates/shared/setup-waas@main
- run: waas validate -f services/services.json
```

## Repository layout

| Path | Contents |
|---|---|
| `.github/workflows/` | The reusable workflows: `waas.yml` (workloads), `iac-workflow-as-a-service.yml` (Terraform); `docs/WAAS.md`, the architecture |
| `workflow-templates/workload/` | Workload actions: loading services, routing, deploying, project templates, GitOps updates, metrics |
| `workflow-templates/infrastructure/` | Terraform actions: setup, plan, apply, security scans |
| `workflow-templates/shared/` | `setup-waas`, `sign-image` and `verify-image` (cosign), Vault and gcloud authentication, Slack, pull request checks |
| `cmd/`, `internal/`, `Makefile` | The `waas` CLI and its build |
| `clients/` | Example caller workflows and services files |

## Development

The `Makefile` runs what CI runs:

| Target | Does |
|---|---|
| `make check` | `gofmt`, `go vet` and the tests: what a pull request must pass |
| `make build` | `bin/waas` for this machine |
| `make dist` | The release binaries and `checksums.txt`, in `dist/` |

To release, push a version tag: `git tag v1.2.3 && git push origin v1.2.3`. The `Release` workflow checks and builds, signs the checksums with cosign (keyless, the workflow's GitHub identity), publishes the GitHub Release, and moves the major tag (`v1`).

To add a project type, add its build steps as an action in `workflow-templates/workload/templates/<project_type>/action.yml` and call it from `deploy` for that `project_type`.
