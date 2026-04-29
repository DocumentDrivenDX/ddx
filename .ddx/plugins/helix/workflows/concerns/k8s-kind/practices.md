# Practices: k8s-kind

## Requirements (Frame phase)
- Identify all services that need to run locally together
- Confirm production target is Kubernetes (not serverless/PaaS)
- Plan for secrets management strategy before first deployment

## Design
- One Helm chart per deployable service
- Chart structure: `Chart.yaml`, `templates/`, `values.yaml` (defaults), `values-dev.yaml`, `values-prod.yaml`
- Use chart dependencies (e.g., bitnami/postgresql) for stateful services
- Service discovery via Kubernetes DNS (`<svc>.<namespace>.svc.cluster.local`)
- Expose services locally via `kubectl port-forward` or kind ingress

## Implementation
- Create local cluster: `kind create cluster --config kind-config.yaml`
- Load local images: `kind load docker-image <image>:<tag>`
- Install/upgrade: `helm upgrade --install <release> ./deploy/helm/<chart> -f values-dev.yaml`
- Image tagging: use git SHA for dev (`$(git rev-parse --short HEAD)`), semver for releases
- Multi-stage Dockerfile: builder stage (full toolchain) → runtime stage (minimal base)
- Do not use `latest` tag in Helm values — pin to a specific tag

## Testing
- Smoke test after `helm install`: run `kubectl get pods` and verify all pods are `Running`
- Integration tests can target services via port-forward
- Use `helm template` to validate rendered manifests before applying

## Quality Gates
- `helm lint ./deploy/helm/<chart>` — chart syntax check
- `helm template ./deploy/helm/<chart> -f values-dev.yaml | kubectl apply --dry-run=client -f -` — manifest validation
- Docker image must build successfully before helm install

## Local Dev Workflow
```bash
kind create cluster --config deploy/kind-config.yaml
docker build -t myapp:local .
kind load docker-image myapp:local
helm upgrade --install myapp ./deploy/helm/myapp -f deploy/helm/myapp/values-dev.yaml
kubectl port-forward svc/myapp 8080:80
```
