# Architecture

How a deployment moves through WaaS, from the calling workflow to the GitOps repository, Slack and the dashboard. The [README](../../../README.md) covers how to use it.

## High-level flow

```mermaid
flowchart TB
 subgraph PlanOutputs["PlanOutputs"]
        E["Environment Config"]
        I["Matrix Services"]
        J["Single Service"]
        L["Staging Config"]
        M["Production Config"]
  end
    A["Caller Workflow"] L_A_B_0@--> B["WaaS Entrypoint"]
    B L_B_D_0@--> D["WaaS Plan<br/>service discovery and routing"]
    D --> H{"Repository Type"} & K{"Branch → Environment"} & E
    H L_H_I_0@-- Monorepo --> I
    H L_H_J_0@-- Polyrepo --> J
    K L_K_L_0@-- dev --> L
    K L_K_M_0@-- main --> M
    PlanOutputs L_PlanOutputs_F_0@--> F["WaaS Deploy"]
    F L_F_G_0@--> G["Notifications"]
    F L_F_N_0@--> N["Metrics Collection"]
    N L_N_O_0@--> O["WaaS Dashboard"]

     A:::Sky
     D:::Plan
     H:::Pine
     K:::Pine
     F:::Deployer
     N:::Metrics
     O:::Dashboard
    classDef Plan stroke-width:2px, stroke:#ff6b6b, fill:#ff6b6b, color:white
    classDef Deployer stroke-width:2px, stroke:#4ecdc4, fill:#4ecdc4, color:white
    classDef Metrics stroke-width:2px, stroke:#9b59b6, fill:#9b59b6, color:white
    classDef Dashboard stroke-width:2px, stroke:#e67e22, fill:#e67e22, color:white
    classDef Pine stroke-width:1px, stroke-dasharray:none, stroke:#254336, fill:#27654A, color:#FFFFFF
    classDef Sky stroke-width:1px, stroke-dasharray:none, stroke:#374D7C, fill:#E2EBFF, color:#374D7C

    L_A_B_0@{ animation: fast } 
    L_B_D_0@{ animation: fast } 
    L_D_K_0@{ animation: fast } 
    L_D_E_0@{ animation: fast } 
    L_H_I_0@{ animation: fast } 
    L_H_J_0@{ animation: fast } 
    L_K_L_0@{ animation: fast } 
    L_K_M_0@{ animation: fast } 
    L_PlanOutputs_F_0@{ animation: fast } 
    L_F_G_0@{ animation: fast }  
    L_F_N_0@{ animation: fast }
    L_N_O_0@{ animation: fast }
```

A service repository calls the reusable workflow, [`waas.yml`](../waas.yml), its entry point. Each stage after it is a composite action:

| Stage | Action | What it does |
|---|---|---|
| Plan | [`plan`](../../../workflow-templates/workload/waas/plan) | Runs `waas plan`: reads the services file (the repository's own, or its entries in the shared catalog), decides whether the repository deploys one service or several, finds the environment the branch deploys to from the file's `metadata.environments` (by default `dev` to staging and `main` to production), and gives deploy that environment's name, domain and build configuration and the services to deploy: a matrix for a monorepo, one service for a polyrepo. |
| Deploy | [`deploy`](../../../workflow-templates/workload/waas/deploy) | Runs once per service, in parallel. It promotes or builds the image, publishes it, signs it when the services file asks (a promotion first verifies the source image), and writes its tag to the service's values in the GitOps repository. |
| Notifications | [`slack-notify`](../../../workflow-templates/shared/slack-notify) | Posts the result to Slack when a deployment ran; skipped and unchanged services send nothing. |
| Metrics collection | [`collect-metrics`](../../../workflow-templates/workload/utils/collect-metrics) | Sends each deployment's record to the WaaS dashboard. |

## Detailed flow

```mermaid
sequenceDiagram
    participant C as Client Repo
    participant W as WaaS Platform
    participant P as Plan
    participant D as Deploy
    participant M as Metrics Collector
    participant DB as WaaS Dashboard
    participant S as Slack
    
    C->>W: project_type, monorepo, dockerfile
    W->>P: Services File & Branch
    P->>P: Load Services & Detect Repo Type
    P->>P: Resolve Environment from Branch
    P->>D: Deployment Instructions
    
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
2. The plan loads the services, decides whether the repository is a monorepo or a polyrepo, and resolves the environment from the branch; a branch no environment takes deploys nothing.
3. Deploy runs per service: a matrix of services for a monorepo, a single service for a polyrepo.
4. For each service it builds, pushes and signs the image, or verifies and promotes the one staging already built, and updates the GitOps repository once the image is signed. A service whose values already hold the tag the run would write is skipped.
5. The deployment's record goes to the dashboard, and Slack is notified only when something was deployed.

## The WaaS dashboard

The dashboard records every deployment WaaS runs and shows them by environment, service, status and time.

`collect-metrics` sends each deployment to `POST <waas_api_url>/api/deployments` with a workflow API key (`X-API-Key`). The record carries the environment, service, service type, status, duration, commit, image tag, and the workflow run it came from. `GET /api/deployments` and `/api/insights` serve the history and its statistics.
