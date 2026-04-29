# Rollback Deployment Prompt

Execute an emergency rollback to restore the system to a known good state when deployment issues are detected.

## Operational Action

This is an emergency action that performs system rollback. It does not generate documentation files.

## Rollback Decision Criteria

### Automatic Triggers
- Error rate > 5% for 5 minutes
- P95 latency > 2x baseline for 10 minutes
- More than 50% health checks failing
- Critical business metrics drop > 20%
- Data corruption or security breach detected

### Manual Triggers
- User reports of critical bugs
- Partial feature failures
- External service incompatibilities
- Business decision (market timing, PR)

## Rollback Strategies

### Kubernetes Rollback
```bash
# Capture current state for post-mortem
kubectl get deployment api -n production -o yaml > deployment-failed-$(date +%s).yaml
kubectl logs -n production -l app=api --tail=1000 > logs-failed-$(date +%s).log

# Rollback
kubectl rollout undo deployment/api -n production
kubectl rollout status deployment/api -n production --timeout=5m

# Rollback to specific revision
kubectl rollout undo deployment/api -n production --to-revision=142
```

### Blue-Green Rollback
Switch load balancer back to previous environment, scale down failed deployment.

### Canary Rollback
Reduce canary traffic to 0%, delete canary deployment.

### Feature Flag Rollback
Disable feature flag for all users -- no deployment needed.

### Database Rollback
If schema changes were made: execute rollback migration in transaction, verify, then restore data from backup if needed.

## Rollback Verification

After rollback completes:
1. Check health endpoints return 200
2. Verify version is the expected previous version
3. Confirm error rate returning to normal
4. Run smoke tests (auth, core functionality)
5. Check business metrics recovering

## Communication

Notify stakeholders immediately with: time detected, severity, impact, actions taken, ETA for resolution. Update status page. Send updates every 15 minutes until resolved.

## Post-Rollback Actions

- **Immediate (30 min)**: Confirm stability, capture logs/metrics, update status page
- **24h**: Root cause analysis, plan fixes, update runbooks
- **1 week**: Implement fixes, blameless post-mortem, plan re-deployment
