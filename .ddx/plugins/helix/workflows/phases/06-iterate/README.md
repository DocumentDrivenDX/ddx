# Phase 06: Iterate

The continuous learning phase where production insights, user feedback, and team experiences transform into actionable improvements for the next cycle.

## Purpose

The Iterate phase closes the HELIX loop by systematically capturing learnings from production deployment and user interaction. This phase emphasizes data-driven decision making, AI-assisted pattern recognition, and continuous improvement. Every iteration makes the next cycle more efficient, higher quality, and better aligned with user needs.

## Key Principle

**Learn, Adapt, Evolve**: Every deployment teaches valuable lessons. Through AI-powered analysis of metrics, feedback, and experiences, we identify patterns, predict issues, and continuously improve both the product and the process.

## Workflow Principles

This phase embodies the HELIX workflow's commitment to:

- **Continuous Learning**: Every data point contributes to organizational knowledge
- **AI-Powered Insights**: Machine learning identifies patterns humans might miss
- **Predictive Improvement**: Anticipate issues before they become problems
- **Human-AI Synthesis**: Combine human intuition with AI analysis
- **Feedback Loop Closure**: Learnings directly influence the next Frame phase

The Iterate phase transforms HELIX from a linear process into a true spiral of continuous improvement.

## Input Gates

Prerequisites to enter this phase (defined in `input-gates.yml`):

1. **System deployed to production**
   - Requirement: Application running in production environment
   - Validation: Health checks passing, monitoring active
   - Source: 05-deploy phase

2. **Monitoring and observability active**
   - Requirement: Metrics, logs, and traces being collected
   - Validation: Dashboards populated with real data
   - Source: 05-deploy phase

3. **Initial user interaction**
   - Requirement: Sufficient usage to generate meaningful data
   - Validation: Minimum threshold of user actions/time passed
   - Source: Production environment

4. **Team availability for retrospective**
   - Requirement: Core team members available for review
   - Validation: Retrospective scheduled and confirmed
   - Source: Team calendar

These gates ensure meaningful data exists for analysis and learning extraction.

## Process Flow

```mermaid
graph TD
    A[Production Deployment] --> B[Collect Metrics & Logs]
    A --> C[Gather User Feedback]
    A --> D[Monitor Performance]
    B --> E[AI Analysis Engine]
    C --> E
    D --> E
    E --> F[Generate Insights]
    F --> G[Team Retrospective]
    G --> H[Prioritize Improvements]
    H --> I[Update Backlog]
    I --> J[Plan Next Iteration]
    J --> K{Continue?}
    K -->|Yes| L[Return to Frame Phase]
    K -->|No| M[Project Complete]
```

## Work Items

### Artifacts

#### Metric Definition
**Artifact Location**: `artifacts/metric-definition/`
**Output Location**: `docs/helix/06-iterate/metrics/*.yaml`

Individual metric specification: name, unit, direction (higher-is-better or
lower-is-better), measurement command, tolerance band, and ratchet floor.

#### Cross-Phase Action: Alignment Review
**Action Location**: `../../actions/reconcile-alignment.md`
**Output Location**: `docs/helix/06-iterate/alignment-reviews/AR-YYYY-MM-DD[-scope].md`

Cross-phase reconciliation review:
- creates or reconciles a review epic and review issues in the tracker
- audits the canonical HELIX stack against implementation evidence
- writes a consolidated alignment report for the review run
- emits follow-up execution issues only where explicit gaps exist

#### Cross-Phase Action: Queue Check
**Action Location**: `../../actions/check.md`
**Output Location**: terminal response only

Bounded execution-state review:
- inspects ready, in-progress, and blocked HELIX work
- checks whether the current scope should implement, align, backfill, wait, ask for guidance, or stop
- returns a deterministic `NEXT_ACTION` code and the exact next command
- should be used when the implementation queue drains instead of looping blindly

#### Cross-Phase Action: Documentation Backfill
**Action Location**: `../../actions/backfill-helix-docs.md`
**Output Location**: `docs/helix/06-iterate/backfill-reports/BF-YYYY-MM-DD[-scope].md`

Research-first documentation reconstruction:
- inventories existing docs, code, tests, CI, and operational evidence
- reconstructs missing HELIX artifacts conservatively from current state
- asks for user guidance before low-confidence canonical content is finalized
- writes a durable backfill report with assumptions, confidence, and follow-up work

#### 9. Security Metrics and Analysis
**Artifact Location**: `artifacts/security-metrics/`
**Output Location**: `docs/helix/06-iterate/security-metrics.md`

Security posture monitoring and improvement tracking:
- **Security incident response metrics (MTTD, MTTR)**
- **Vulnerability management and remediation tracking**
- **Compliance monitoring and audit findings analysis**
- **Security training effectiveness and awareness metrics**
- **Threat landscape analysis and defense effectiveness**
- **Security improvement backlog prioritization and planning**

**AI Capabilities**:
- Security trend analysis and pattern recognition
- Threat correlation and risk assessment
- Automated compliance monitoring and reporting
- Security control effectiveness measurement

## Artifact Metadata

Each artifact directory includes a `meta.yml` file that defines:
- **Data Sources**: Metrics, logs, feedback channels
- **Analysis Frequency**: Real-time, daily, weekly, per-iteration
- **AI Models Used**: Specific ML models for analysis
- **Automation Level**: Full, semi, or manual processing
- **Integration Points**: How insights feed back into workflow


## Human vs AI Responsibilities

### Human Responsibilities
- **Strategic Decisions**: Determine product direction
- **Stakeholder Communication**: Manage expectations and relationships
- **Creative Problem Solving**: Innovate solutions to complex issues
- **Team Morale**: Maintain team health and motivation
- **Final Prioritization**: Make trade-off decisions

### AI Assistant Responsibilities
- **Data Analysis**: Process large volumes of metrics and logs
- **Pattern Recognition**: Identify trends and correlations
- **Anomaly Detection**: Alert on unusual patterns
- **Prediction**: Forecast future issues and opportunities
- **Report Generation**: Synthesize insights into actionable reports
- **Recommendation Engine**: Suggest improvements based on data

## Quality Gates

Before proceeding to the next Frame phase, ensure:

### Analysis Completeness
- [ ] All production metrics analyzed
- [ ] User feedback synthesized
- [ ] Performance baselines established
- [ ] Incidents reviewed and documented
- [ ] Team retrospective completed

### Learning Extraction
- [ ] Patterns identified across data sources
- [ ] Lessons learned documented
- [ ] Success factors understood
- [ ] Failure modes analyzed
- [ ] Knowledge base updated

### Planning Readiness
- [ ] Improvement backlog prioritized
- [ ] Next iteration goals defined
- [ ] Resource allocation planned
- [ ] Risk mitigation strategies identified
- [ ] Success metrics established

## Common Pitfalls

### ❌ Avoid These Mistakes

1. **Analysis Paralysis**
   - Bad: Endless analysis without action
   - Good: Time-boxed analysis with clear decisions

2. **Ignoring Negative Feedback**
   - Bad: Cherry-picking positive metrics
   - Good: Honest assessment of all feedback

3. **Skipping Retrospectives**
   - Bad: Moving to next iteration without reflection
   - Good: Dedicated time for team learning

4. **Over-Reacting to Anomalies**
   - Bad: Major pivots based on outliers
   - Good: Statistical significance before changes

5. **Knowledge Silos**
   - Bad: Learnings stay with individuals
   - Good: Documented, shared knowledge base

## Success Criteria

The Iterate phase is complete when:

1. **Data Analyzed**: All metrics, logs, and feedback processed
2. **Insights Generated**: Clear learnings extracted from data
3. **Improvements Identified**: Prioritized backlog of enhancements
4. **Team Aligned**: Retrospective completed with follow-up issues identified
5. **Next Cycle Planned**: Clear goals for next iteration
6. **Knowledge Captured**: Learnings documented for future reference

## Continuous Improvement Metrics

Track these metrics across iterations to measure improvement:

### Product Metrics
- **User Satisfaction**: NPS, CSAT trends
- **Performance**: Response time improvements
- **Quality**: Defect rates, incident frequency
- **Adoption**: User growth, feature usage

### Process Metrics
- **Velocity**: Story points per iteration
- **Cycle Time**: Frame to Deploy duration
- **Defect Escape Rate**: Bugs found in production
- **Automation Coverage**: % of automated tests/deployments

### Team Metrics
- **Team Health**: Satisfaction and engagement
- **Knowledge Sharing**: Documentation quality
- **Skill Development**: New capabilities acquired
- **Collaboration**: Cross-functional effectiveness

## Analysis Tools

Iterate work is driven by the cross-phase actions (`helix align`, `helix review`,
`helix experiment`) and metric definitions in `artifacts/metric-definition/`.

## Integration with Next Cycle

The Iterate phase outputs directly influence the next Frame phase:

### Feedback → Requirements
- User feedback becomes new user stories
- Performance issues become NFRs
- Feature requests become product requirements

### Learnings → Design
- Technical lessons inform architecture decisions
- Performance insights guide optimization
- Security findings strengthen design

### Metrics → Success Criteria
- Current baselines become future targets
- Trend analysis sets realistic goals
- Cost data influences scope decisions

## Tips for Success

1. **Automate Data Collection**: Set up comprehensive monitoring before deployment
2. **Regular Analysis Cadence**: Don't wait until iteration end to analyze
3. **Cross-Functional Participation**: Include all roles in retrospectives
4. **Action-Oriented Insights**: Every learning should have an action
5. **Celebrate Successes**: Recognize what worked well
6. **Fail Fast, Learn Faster**: Treat failures as learning opportunities
7. **Document Everything**: Future you will thank current you

## Using AI Assistance

Iterate work is decided through `helix check` and the canonical cross-phase
actions:
- `.ddx/plugins/helix/workflows/actions/reconcile-alignment.md`
- `.ddx/plugins/helix/workflows/actions/backfill-helix-docs.md`

For metric tracking, use `artifacts/metric-definition/`.

AI is useful for synthesis, clustering feedback, and surfacing patterns. Human
judgment remains responsible for prioritization, tradeoffs, and scheduling.


## File Organization

### Structure Overview
- **Analysis Artifacts**: `.ddx/plugins/helix/workflows/phases/06-iterate/artifacts/`
  - Templates for capturing and analyzing learnings
  - Prompts for AI-assisted insight generation

- **Generated Insights**: `docs/helix/06-iterate/`
  - Completed analyses and reports
  - Lessons learned documentation
  - Planning documents for next iteration

This separation keeps analysis templates reusable while organizing insights where they're most valuable for the team.

---

*The Iterate phase transforms each ending into a new beginning, ensuring every cycle builds on the learnings of the last. This is where the HELIX spiral ascends.*
