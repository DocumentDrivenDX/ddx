# Deploy to Production Prompt

Execute a controlled, monitored deployment to production with progressive rollout and rollback capabilities.

## Operational Action

This action performs production deployment. It does not generate documentation files.

## Pre-Deployment Go/No-Go

All items must be checked before proceeding:
- [ ] Staging deployment successful for 24+ hours
- [ ] All stakeholders notified and approved
- [ ] On-call team ready, monitoring dashboards open
- [ ] Rollback plan tested in staging
- [ ] Recent production backup verified

## Deployment Strategies

Choose the appropriate strategy:

### Blue-Green
1. Deploy to green environment, verify health
2. Switch load balancer from blue to green
3. Keep blue scaled down but ready for instant rollback

### Canary
1. Deploy to canary (5% traffic), monitor 30 min
2. Increase to 25%, then 50%, then 100% with monitoring at each stage
3. Compare canary error rate and latency against stable baseline

### Feature Flags
1. Deploy code with features disabled
2. Enable progressively: internal users -> 10% -> 50% -> 100%

## Execution Steps

1. **Pre-flight**: Verify production health, check resource availability, confirm external dependencies
2. **Database migration** (if needed): Backup first, run in transaction, verify
3. **Deploy**: Apply configuration, update image, monitor rollout status
4. **Immediate validation** (0-5 min): Health endpoints, version check, smoke tests
5. **Extended monitoring** (5-30 min): Watch error rate (<1%), latency (within 10% of baseline), throughput

## Rollback Triggers

Initiate rollback if:
- Error rate > 5% for 5 minutes
- P95 latency > 2x baseline for 10 minutes
- Critical functionality broken
- Data corruption detected

## Rollback

```bash
# Kubernetes rollback
kubectl rollout undo deployment/api -n production
# Blue-green: switch back
kubectl patch service production-lb -n production -p '{"spec":{"selector":{"deployment":"blue"}}}'
```

## Post-Deployment

- **First hour**: Monitor dashboards, review error logs, test critical paths
- **First day**: Analyze performance, review user feedback, schedule retrospective

## Success Criteria

- Zero downtime, error rate < 0.1%, P95 latency within 10% of baseline
- No critical alerts, no emergency rollback, business metrics stable
