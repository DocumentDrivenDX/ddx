---
dun:
  id: helix.architecture
  depends_on:
    - helix.prd
---
# Architecture Diagrams

## Level 1: System Context

```mermaid
graph TB
    %% [Add users, system, external dependencies]
```

| Element | Type | Purpose | Protocol |
|---------|------|---------|----------|
| [User/System] | User/External | [Interaction] | [HTTP/API/etc] |

## Level 2: Container Diagram

```mermaid
graph TB
    %% [Add containers: Web, API, DB, Cache, Queue, Worker]
```

| Container | Technology | Responsibilities | Communication |
|-----------|------------|------------------|---------------|
| [Name] | [Stack] | [What it does] | [Protocol/Format] |

## Level 3: Component Diagram

```mermaid
graph TB
    %% [Add components per container: Controller, Service, Repository, etc]
```

| Component | Purpose | Implementation Notes |
|-----------|---------|---------------------|
| [Name] | [Responsibility] | [Key decisions] |

## Deployment

```mermaid
graph TB
    %% [Add: LB, Web Tier, App Tier, Data Tier]
```

| Component | Infrastructure | Instances | Scaling |
|-----------|---------------|-----------|---------|
| [Name] | [Container/VM] | [Count] | [H/V] |

## Data Flow

```mermaid
sequenceDiagram
    %% [Add sequence for key use case]
```

## Architecture Summary

| Layer | Technology | Rationale |
|-------|------------|-----------|
| Frontend | [Tech] | [Why] |
| Backend | [Tech] | [Why] |
| Database | [Tech] | [Why] |
| Infra | [Tech] | [Why] |

**Patterns**: [Pattern 1: usage] | [Pattern 2: usage]

## Quality Attributes

| Attribute | Strategy | Key Decisions |
|-----------|----------|---------------|
| Scalability | [H/V scaling] | [Approach] |
| Security | [Controls] | [Boundaries] |
| Observability | [Metrics/Logging/Tracing] | [Tools] |
| Disaster Recovery | RTO: [target] / RPO: [target] | [Backup/Failover strategy] |
