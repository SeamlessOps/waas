# Examples

## minimal

The smallest setup: a service repository calls WaaS's workflow, and one services file in the GitOps repository says what each repository deploys.

- [`deploy.yml`](minimal/deploy.yml) is the whole workflow a service repository needs.
- [`services/services.json`](minimal/services/services.json) declares two environments (`dev` to staging, `main` to production), defaults for two repositories, three services, and keyless image signing. `shop-api` deploys from `develop` instead of `dev`.

## exampleorg

A real platform, renamed. ExampleOrg runs about twenty services from twelve repositories: a .NET monorepo with three services, an Nx monorepo with eight frontends, and a repository per service for the rest. Every one deploys the same way, and none of them carries deployment logic.

**The GitOps repository owns the workflow.** `ExampleOrg/exampleorg-products` keeps its own copy of WaaS's workflow, [`waas.yml`](exampleorg/waas.yml), with the organization's defaults: its GitOps repository, its Argo CD link (`dashboard_url`, one Argo CD application per values file), its package feeds. It calls WaaS's actions at `@v0`. A service repository's workflow is a few lines that call this one; changing how ExampleOrg deploys is a change in one place.

**One catalog describes every service.** [`services/services.json`](exampleorg/services/services.json) lists each service under its repository. A repository with one entry deploys as a single service; one with several deploys them all, in parallel. Shared settings sit once under `repositories`: the .NET monorepo's SDK version and the databases it needs, the frontends' Node version. Each entry adds only what differs: its directory, its Nx project, its own environments.

**Branches pick the environment.** A merge into `dev` deploys to staging, built as `Release`; a merge into `main` deploys to production, built as `Production`. Each environment has its own domain, which completes a service's values key (`api-exampleorg-dev`).

**Production reuses staging's image.** When `main` receives the code staging already runs, production gets staging's image, verified and copied, instead of a new build.

**Every image is signed.** Images are signed with a Cloud KMS key (`metadata.signing.key`): before an image's tag reaches the GitOps repository, and again when it is promoted. A promotion verifies the source image first. Nothing goes to a public transparency log, and a cluster admission policy can check the same key.

`my-gcp-project` stands in for the real project.
