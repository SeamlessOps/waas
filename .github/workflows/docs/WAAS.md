# WaaS architecture

How a deployment moves through WaaS, from the calling workflow to the GitOps repository, Slack and the dashboard. The [README](../../../README.md) covers how to use it.

## High-level flow

```mermaid
flowchart TB
 subgraph RouterOutputs["RouterOutputs"]
        E["Environment Config"]
        I["Matrix Services"]
        J["Single Service"]
        L["Staging Config"]
        M["Production Config"]
  end
    A["Caller Workflow"] L_A_B_0@--> B["WaaS Entrypoint"]
    B L_B_C_0@--> C["Service Discovery"]
    C L_C_D_0@--> D["WaaS Router"]
    D --> H{"Repository Type"} & K{"Branch Pattern"} & E
    H L_H_I_0@-- Monorepo --> I
    H L_H_J_0@-- Polyrepo --> J
    K L_K_L_0@-- dev --> L
    K L_K_M_0@-- main --> M
    RouterOutputs L_RouterOutputs_F_0@--> F["WaaS Deployer"]
    F L_F_G_0@--> G["Notifications"]
    F L_F_N_0@--> N["Metrics Collection"]
    N L_N_O_0@--> O["WaaS Dashboard"]

     A:::Sky
     D:::Router
     H:::Pine
     K:::Pine
     F:::Deployer
     N:::Metrics
     O:::Dashboard
    classDef Router stroke-width:2px, stroke:#ff6b6b, fill:#ff6b6b, color:white
    classDef Deployer stroke-width:2px, stroke:#4ecdc4, fill:#4ecdc4, color:white
    classDef Metrics stroke-width:2px, stroke:#9b59b6, fill:#9b59b6, color:white
    classDef Dashboard stroke-width:2px, stroke:#e67e22, fill:#e67e22, color:white
    classDef Pine stroke-width:1px, stroke-dasharray:none, stroke:#254336, fill:#27654A, color:#FFFFFF
    classDef Sky stroke-width:1px, stroke-dasharray:none, stroke:#374D7C, fill:#E2EBFF, color:#374D7C

    L_A_B_0@{ animation: fast } 
    L_B_C_0@{ animation: fast } 
    L_C_D_0@{ animation: fast } 
    L_D_K_0@{ animation: fast } 
    L_D_E_0@{ animation: fast } 
    L_H_I_0@{ animation: fast } 
    L_H_J_0@{ animation: fast } 
    L_K_L_0@{ animation: fast } 
    L_K_M_0@{ animation: fast } 
    L_RouterOutputs_F_0@{ animation: fast } 
    L_F_G_0@{ animation: fast }  
    L_F_N_0@{ animation: fast }
    L_N_O_0@{ animation: fast }
```

A service repository calls the reusable workflow, [`waas.yml`](../waas.yml), its entry point. Each stage after it is a composite action:

| Stage | Action | What it does |
|---|---|---|
| Service discovery | [`load-services-matrix`](../../../workflow-templates/workload/utils/load-services-matrix) | Reads the services file, the repository's own or its entries in the shared catalog, and decides whether the repository deploys one service or several, and which deploy to each environment. The decision is made by the `waas` CLI. |
| Router | [`router`](../../../workflow-templates/workload/waas/router) | Finds the environment the branch deploys to, from the services file's `metadata.environments` (by default `dev` to staging and `main` to production), and gives the deployer that environment's name, domain and build configuration, and the services to deploy: a matrix for a monorepo, one service for a polyrepo. |
| Deployer | [`deployer`](../../../workflow-templates/workload/waas/deployer) | Runs once per service, in parallel. It promotes or builds the image, publishes it, and writes its tag to the service's values in the GitOps repository. |
| Notifications | [`slack-notify`](../../../workflow-templates/shared/slack-notify) | Posts the result to Slack when a deployment ran; skipped and unchanged services send nothing. |
| Metrics collection | [`collect-metrics`](../../../workflow-templates/workload/utils/collect-metrics) | Sends each deployment's record to the WaaS dashboard. |

## Detailed flow

```mermaid
sequenceDiagram
    participant C as Client Repo
    participant W as WaaS Platform
    participant R as Router
    participant D as Deployer
    participant M as Metrics Collector
    participant DB as WaaS Dashboard
    participant S as Slack
    
    C->>W: project_type, monorepo, dockerfile
    W->>W: Load Services & Detect Repo Type
    W->>R: Route by Branch & Environment
    R->>R: Resolve Environment Config
    R->>D: Deployment Instructions
    
    alt Monorepo Detected
        D->>D: Matrix Deployment<br/>Multiple Services
    else Polyrepo Detected
        D->>D: Single Service<br/>Focused Deployment
    end
    
    D->>D: Build, Push & Update GitOps
    D->>M: Deployment Metrics
    M->>DB: Store Metrics
    D->>S: Conditional Notification
    S->>S: Notify Only on Changes
```

1. The caller passes its project type and repository settings; the services file supplies the rest per service.
2. WaaS loads the services and decides whether the repository is a monorepo or a polyrepo.
3. The router resolves the environment from the branch; a branch no environment takes deploys nothing.
4. The deployer runs per service: a matrix of services for a monorepo, a single service for a polyrepo.
5. For each service it builds and pushes the image, or promotes the one staging already built, and updates the GitOps repository. A service whose values already hold the tag the run would write is skipped.
6. The deployment's record goes to the dashboard, and Slack is notified only when something was deployed.

## The WaaS dashboard

The dashboard records every deployment WaaS runs and shows them by environment, service, status and time.

`collect-metrics` sends each deployment to `POST <waas_api_url>/api/deployments` with a workflow API key (`X-API-Key`). The record carries the environment, service, service type, status, duration, commit, image tag, and the workflow run it came from. `GET /api/deployments` and `/api/insights` serve the history and its statistics.
