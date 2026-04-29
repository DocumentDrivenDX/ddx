# Concern: Kubernetes + kind

## Category
infrastructure

## Areas
infra

## Components

- **Local cluster**: `kind` (Kubernetes in Docker) — NOT docker-compose
- **Package manager**: Helm for application deployment
- **Manifests**: Helm charts with `values.yaml`, `values-dev.yaml`, `values-prod.yaml`
- **Image builds**: Docker with multi-stage builds; image tagged from git SHA or semver
- **Local dev workflow**: `kind create cluster` + `helm install` + port-forward

## Constraints

- Local development uses a kind cluster, not docker-compose
- Services packaged as Helm charts with environment-specific values files
- Image builds must be reproducible (deterministic tags, no `latest` in production)
- Secrets managed via Kubernetes Secrets or external secret manager — not in values files
- `values-dev.yaml` overrides for local kind cluster; `values-prod.yaml` for production

## When to use

Projects with services that deploy to Kubernetes in production. Kind provides
a local cluster that mirrors production closely enough to catch config and
networking issues before deployment. Prefer kind over docker-compose when
services need service discovery, ingress, or multi-container orchestration.

## ADR References
