<div align="center">
  <img src="./assets/waas.svg" alt="WaaS, Workflow as a Service" height="64" />

  <p><strong>Push a branch. Its services ship.</strong></p>

  <p>
    <a href="https://github.com/rollwright/waas/actions/workflows/go.yml"><img src="https://github.com/rollwright/waas/actions/workflows/go.yml/badge.svg" alt="CI" /></a>
    <a href="https://github.com/rollwright/waas/releases"><img src="https://img.shields.io/github/v/release/rollwright/waas?label=version" alt="Version" /></a>
    <img src="https://img.shields.io/badge/Go-1.24+-00ADD8.svg" alt="Go 1.24+" />
    <img src="https://img.shields.io/badge/GitHub%20Actions-reusable%20workflow-2088FF.svg" alt="A reusable GitHub Actions workflow" />
    <img src="https://img.shields.io/badge/images-signed%20with%20cosign-0D9488.svg" alt="Images signed with cosign" />
  </p>
</div>

Reusable GitHub Actions workflows that build services and deploy them through a GitOps repository.

```yaml
jobs:
  deploy:
    uses: rollwright/waas/.github/workflows/waas.yml@v0
    with:
      project_type: nx
    secrets: inherit
```

A service repository calls one workflow. WaaS works out which services to deploy and to which environment, builds and publishes their images, signs them, and writes the new images into the GitOps repository, where a GitOps controller such as Argo CD picks them up. It also has a workflow for Terraform: lint, security scans, plan and apply.

## Why

Every team that deploys through GitOps ends up writing the same pipeline. Work out the environment from the branch. Build the image. Push it. Clone the GitOps repository, find the right values file, change one tag, commit, retry on a conflict. Each repository gets its own copy, the copies drift, and a fix in one never reaches the others.

WaaS is that pipeline written once. What differs between repositories (which services they have, where each deploys, how each builds) lives in one services file in the GitOps repository, not in each repository's workflow. The decisions are made by a tested Go program, [`waas`](#the-waas-cli), not by shell in YAML.

## What WaaS handles

A few of the things WaaS accounts for, so a service repository doesn't have to:

- **The branch decides the environment.** `dev` deploys to staging and `main` to production, unless the services file says otherwise, per file or per repository. A branch no environment takes deploys nothing.
- **One repository, many services.** A monorepo deploys each of its services in parallel; a polyrepo deploys as one. A repository without a services file of its own is looked up in a shared catalog.
- **Re-runs don't redeploy.** A service whose values already hold the tag this run would write is skipped.
- **Production doesn't rebuild what staging built.** When nothing changed between the commit staging's image came from and the one being deployed, the image is promoted, not rebuilt.
- **Nothing unsigned lands.** With signing on, an image is signed before its tag is written, and a promotion verifies the source's signature first. An image without one is built again.
- **Only the image changes.** In the GitOps repository, WaaS writes only the service's image fields, in the one values file that holds it: the tag alone by default, or the repository, the full image or the digest where [`values`](#where-the-image-is-written) says. Nothing else in the file is touched.
- **Every problem at once.** `waas validate` reports everything wrong with a services file, not only the first thing. The file can be YAML or JSON.

## How a deployment runs

[The architecture](.github/workflows/docs/architecture.md) shows how a deployment flows, in diagrams.

The workload workflow, [`.github/workflows/waas.yml`](.github/workflows/waas.yml), runs in four stages; a push from a merged pull request deploys, and its branch decides the environment:

1. **Validate.** On a pull request, the workflow runs the pull request checks and stops there.
2. **Plan.** [`plan`](workflow-templates/workload/waas/plan) runs `waas plan`: it reads the services file, finds the environment the branch deploys to (from the file's `metadata.environments`, by default `dev` to staging and `main` to production), and gives that environment's name, domain and build configuration and the services that deploy there. A branch no environment takes deploys nothing.
3. **Deploy** each service, in parallel, with [`deploy`](workflow-templates/workload/waas/deploy):
   - reads its credentials from Vault;
   - skips the service when its values already hold the tag this run would write, as on a re-run;
   - builds the project with the template for its project type, for `dotnet` and `nx`;
   - builds and publishes the Docker image, and signs it when the services file asks;
   - writes the image (its tag, or the fields [`values`](#where-the-image-is-written) names) to the service's values file in the GitOps repository, only once the image is signed;
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
    uses: rollwright/waas/.github/workflows/waas.yml@v0
    with:
      project_type: dotnet
    permissions:
      id-token: write
      contents: read
    secrets: inherit
```

The repository needs the secrets `VAULT_ADDR`, `VAULT_ROLE` and `VAULT_SECRET_PATH`: Vault holds the tokens for the GitOps repository and the registry.

An organization can also keep its own copy of `waas.yml` in its GitOps repository, with its own defaults, and have its service repositories call that. The copy uses WaaS's actions as they are.

Reference a release: `@v0` follows the latest `v0.x.y`, or pin an exact version such as `@v0.1.0`. [Releases](https://github.com/rollwright/waas/releases) lists them.

### Inputs

| Input | Default | Description |
|---|---|---|
| `project_type` | required | `dotnet` (or `.net`) and `nx` build with their template before the image; `generic`, `node`, `python` and `java` build the image only. A service in the services file can set its own |
| `monorepo` | `true` | The repository deploys several services |
| `service` | the repository's name | The service a polyrepo deploys as |
| `services_dir` | `.github/workflows/services` | Directory of the services files, in the GitOps repository |
| `services_file` | `<repository>` | The repository's services file: `<repository>.yaml`, `.yml` or `.json`, whichever exists |
| `services_catalog` | `services` | The shared catalog, in `services_dir`, a repository without its own services file is looked up in. Name it in full (`services.yaml`) when files in both formats sit beside each other |
| `load_default` | `true` | Read the services file from the GitOps repository; `false` reads it from the calling repository |
| `dockerfile` | `Dockerfile` | Dockerfile to build |
| `dotnet_version` | `9.0.x` | .NET SDK version |
| `node_version` | `22.2.x` | Node.js version |
| `dashboard_url` | none | Link to a deployment in the GitOps controller, shown in the run's summary. `{environment}` and `{app}` are filled in; `{app}` is the values file the service is in (`environment/<environment>/<app>.yaml`), so one Argo CD application per file links as `https://argocd.example.com/applications/argocd/apps-{app}-{environment}` |
| `dry-run` | `false` | Plan without deploying |

## The services file

A services file lists what a repository deploys. It lives in the GitOps repository, in `services_dir`, either as the repository's own file (`<repository>.yaml` or `<repository>.json`) or as entries in a shared catalog (`services.yaml` or `services.json`) that many repositories use. It is YAML or JSON, by its extension. Where a path is asked for, the extension may be left off: `services` reads whichever of `services.yaml`, `services.yml` and `services.json` exists, and it is an error for more than one to.

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

Without `metadata.environments`, or without a services file, there are two: staging on `dev` and production on `main`, each named as its domain and built as `Release`. [`examples`](examples/) has complete files.

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

By default, each environment's values are in `environment/<environment>/`, as YAML files with a `services` map. A service's image tag is `services.<name>-<domain>.tag`, in exactly one file of its environment. WaaS changes only that field: the chart knows the image's repository.

### Where the image is written

`values` says where else to write, and what. Set it on an environment (`metadata.environments[].values`), a repository (`repositories.<repository>.values`) or a service entry; each is laid over the one before, and over the default:

```yaml
values:
  path: environment/{environment}   # a values file, or a directory with exactly one file holding the service
  key: services.{name}-{domain}     # the service's dotted path in that file
  image:
    tag: tag                        # the fields written, relative to key
```

`image` names the fields WaaS writes. A layer that sets `image` replaces the one below it whole:

| `image` | Writes | For |
|---|---|---|
| `{tag: tag}` | `build-12-sha-0bf6e22` | The default: a chart that sets the repository itself |
| `{repository: image.repository, tag: image.tag}` | `us-docker.pkg.dev/p/r/api-staging` and the tag | The common Helm shape, `image.repository` and `image.tag` |
| `{ref: image}` | `us-docker.pkg.dev/p/r/api-staging:build-12-sha-0bf6e22` | Plain manifests, kustomize, or a chart that takes the whole image |
| `digest: image.digest`, beside any of them | `sha256:…` | Running exactly the image that was signed, whatever happens to the tag |

`path` and `key` take `{environment}`, `{name}` and `{domain}`. Paths stay inside the GitOps repository, and keys are dotted paths of letters, digits, `_` and `-`; `waas validate` checks both. A repository laid out differently:

```yaml
repositories:
  legacy:
    values:
      path: clusters/{environment}/legacy.yaml
      key: apps.{name}
      image: { repository: image.repository, tag: image.tag }
```

`waas plan` gives each service the result, with placeholders filled in, as `gitops` (where its image is written) and `gitops_from` (where it is read in the environment it promotes from); pass them to `deploy` as `gitops` and `promote_from_gitops`. Without them, `deploy` writes the default. A re-run skips the build when every field already holds this run's image; the digest is not compared, since it is known only after the build.

## Promotion

Deploy can promote instead of building: when production deploys code that staging already built, it copies staging's image to the production name and writes production's tag. It does so when the comparison between the commit staging's tag was built from and the commit being deployed lists no changed files; otherwise it builds. The plan gives the environment to promote from (`promote_from_environment`, `promote_from_domain`: the one before in `metadata.environments`); the calling workflow enables promotion by passing them to `deploy` as `promote_from_environment` and `promote_from_service` (`<name>-<domain>`). A service opts out with `"promote": false`. The source environment's image is read the way that environment is written, from its own `values`. Images must take their settings at runtime for this to be safe. With [image signing](#image-signing), only an image with a valid signature is promoted.

## Metrics and the dashboard

Every deployment is recorded on the WaaS dashboard: its environment, service, status, duration, commit, image tag and workflow run. `collect-metrics` sends the record to `POST <waas_api_url>/api/deployments` with a workflow API key. The dashboard shows the history and its statistics by environment, service, status and time.

## Infrastructure

[`.github/workflows/iac-workflow-as-a-service.yml`](.github/workflows/iac-workflow-as-a-service.yml) runs Terraform for a working directory:

- On a pull request: TFLint and tfsec, then `terraform plan`, the plan commented on the pull request, and a Checkov scan of the plan. Failed scans are posted to Slack, and a failed plan gets an AI explanation.
- After merge: `terraform apply`, with the result posted to Slack.

## The `waas` CLI

The work on data (reading the services file, deciding what a repository deploys, routing a branch) is a Go program in [`cmd/waas`](cmd/waas), not shell:

```bash
waas validate -f services
waas matrix -f services/shop --catalog services/services --repository shop
waas plan -f services/shop --catalog services/services --repository shop --branch dev
```

- `validate` checks a services file and reports every problem.
- `plan` is what a push deploys: the environment its branch takes (name, domain, build configuration), the services there, and the environment it can promote from. Everything comes from the services file; the flags only say where it is, which repository asks, and the branch (`$GITHUB_REF_NAME` by default).
- `matrix` prints what a repository deploys in every environment.

Inside GitHub Actions, each also sets the step's outputs and summary.

Each release publishes the CLI's binaries for Linux and macOS (amd64, arm64) with signed checksums. The actions get it through [`setup-waas`](workflow-templates/shared/setup-waas), which downloads the binary for the runner, verifies the checksums' signature (made by this repository's release workflow, at a version tag) and the binary, and puts `waas` on `PATH`; `version` pins a release, and the default is the latest. To use it in your own workflow:

```yaml
- uses: rollwright/waas/workflow-templates/shared/setup-waas@v0
- run: waas validate -f services/services
```

## Repository layout

| Path | Contents |
|---|---|
| `.github/workflows/` | The reusable workflows: `waas.yml` (workloads), `iac-workflow-as-a-service.yml` (Terraform); `docs/architecture.md` |
| `workflow-templates/workload/` | Workload actions: loading services, routing, deploying, project templates, GitOps updates, metrics |
| `workflow-templates/infrastructure/` | Terraform actions: setup, plan, apply, security scans |
| `workflow-templates/shared/` | `setup-waas`, `sign-image` and `verify-image` (cosign), Vault and gcloud authentication, Slack, pull request checks |
| `cmd/`, `internal/`, `Makefile` | The `waas` CLI and its build |
| `examples/` | A minimal setup, and a real platform's workflow and catalog |
| `assets/` | The logo, `waas.svg` |

## Development

The `Makefile` runs what CI runs:

| Target | Does |
|---|---|
| `make check` | `gofmt`, `go vet` and the tests: what a pull request must pass |
| `make build` | `bin/waas` for this machine |
| `make dist` | The release binaries and `checksums.txt`, in `dist/` |

To release, push a version tag: `git tag v1.2.3 && git push origin v1.2.3`. The `Release` workflow checks and builds, signs the checksums with cosign (keyless, the workflow's GitHub identity), publishes the GitHub Release, and moves the major tag (`v1`).

To add a project type, add its build steps as an action in `workflow-templates/workload/templates/<project_type>/action.yml` and call it from `deploy` for that `project_type`.

## Status

WaaS is before 1.0. Inputs and services-file fields may still change between minor versions; release notes list every breaking change. Pin `@v0` to follow `v0.x.y`, or an exact version.

## Contributing

WaaS grows by what deployments have taught the hard way. If a push deployed somewhere it shouldn't, or you've written the same workflow step into a second repository, that's a contribution: a fix with a test that shows the case, or a new option.

- Decisions about data (what deploys, where, from which file) belong in the `waas` CLI, in Go, with tests. Actions run the CLI and the tools; they don't decide.
- A new services-file field is documented under [The services file](#the-services-file) and checked by `waas validate`.
- `make check` must pass. See [Development](#development).
