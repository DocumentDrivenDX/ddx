# HELIX Action: Analyze Metrics

You are a HELIX Iterate phase executor tasked with systematically analyzing performance, usage, and quality metrics to drive continuous improvement. Your role is to extract actionable insights from data to inform future iterations.

## Action Purpose

Analyze collected metrics to identify improvement opportunities, validate assumptions, and guide decision-making for the next development cycle.

## When to Use This Action

- After feature deployment and initial usage period
- When metrics collection has sufficient data
- Before planning next iteration improvements
- When performance or quality issues need investigation
- During regular retrospective cycles

## Prerequisites

- [ ] Metrics collection systems in place
- [ ] Sufficient data collected (minimum 2 weeks)
- [ ] Baseline metrics established
- [ ] Analytics tools configured
- [ ] Stakeholder success criteria defined

## Action Workflow

### 1. Metrics Collection and Validation

**Data Gathering Checklist**:
```
📊 METRICS ANALYSIS SESSION

1. PERFORMANCE METRICS
   - Response times (P50, P95, P99)
   - Throughput (requests per second)
   - Error rates and types
   - System resource utilization
   - Database query performance

2. USER BEHAVIOR METRICS
   - Active users (daily, weekly, monthly)
   - Feature adoption rates
   - User journey completion rates
   - Session duration and depth
   - Churn and retention rates

3. BUSINESS METRICS
   - Key performance indicators (KPIs)
   - Conversion rates
   - Revenue impact
   - Cost per transaction
   - Customer satisfaction scores

4. QUALITY METRICS
   - Bug discovery rate
   - Time to resolution
   - Code quality scores
   - Test coverage
   - Technical debt metrics
```

### 2. Metric Analysis Framework

**Analysis Structure**:
```markdown
## Metric Analysis: [Metric Category]

### Current State
**Measurement Period**: [Start Date] to [End Date]
**Data Source**: [Analytics platform/tool]
**Sample Size**: [Number of data points]

### Key Findings

#### Metric: [Specific Metric Name]
- **Current Value**: [Value with units]
- **Target Value**: [Goal/benchmark]
- **Trend**: [Improving/Declining/Stable]
- **Variance**: [How much variation observed]

#### Performance Analysis
- **Best Performance**: [When/what conditions]
- **Worst Performance**: [When/what conditions]
- **Pattern Recognition**: [Daily/weekly/seasonal patterns]
- **Anomaly Detection**: [Unusual spikes or drops]

### Comparative Analysis
- **Period-over-Period**: [Comparison with previous period]
- **Target vs. Actual**: [How close to goals]
- **Benchmark Comparison**: [Industry standards]
- **Feature Correlation**: [Impact of recent changes]

### Root Cause Analysis
**High-Performing Areas**:
- Factor 1: [What's working well]
- Factor 2: [Contributing success elements]

**Underperforming Areas**:
- Issue 1: [What's not working]
- Issue 2: [Areas needing improvement]
- Root Cause: [Underlying reasons]
```

### 3. Data Deep Dive

**User Behavior Analysis**:
```markdown
## User Journey Analysis

### Feature Adoption Funnel
1. **Awareness**: [% who discover feature]
2. **Trial**: [% who try feature]
3. **Adoption**: [% who use regularly]
4. **Mastery**: [% who use advanced features]

### Drop-off Analysis
- **Step 1 → 2**: [X% drop-off, reasons: ...]
- **Step 2 → 3**: [Y% drop-off, reasons: ...]
- **Step 3 → 4**: [Z% drop-off, reasons: ...]

### User Segments
#### Power Users (top 10%)
- Characteristics: [What defines them]
- Behavior patterns: [How they use the system]
- Value delivered: [What they accomplish]

#### Regular Users (middle 80%)
- Usage patterns: [Typical behavior]
- Pain points: [Where they struggle]
- Improvement opportunities: [What would help]

#### Struggling Users (bottom 10%)
- Common issues: [What blocks them]
- Support needs: [What assistance required]
- Retention risks: [Likelihood to churn]
```

**Performance Deep Dive**:
```markdown
## Performance Analysis

### Response Time Analysis
- **P50 Response Time**: [Median performance]
- **P95 Response Time**: [95th percentile]
- **P99 Response Time**: [99th percentile]

### Performance by Endpoint
| Endpoint | Avg Response | P95 | Error Rate | Volume |
|----------|--------------|-----|------------|---------|
| /api/users | 120ms | 250ms | 0.1% | 10K/day |
| /api/data | 450ms | 1.2s | 0.5% | 5K/day |

### Infrastructure Utilization
- **CPU Usage**: [Average/peak percentages]
- **Memory Usage**: [Current consumption patterns]
- **Database Load**: [Query performance and load]
- **Network I/O**: [Bandwidth utilization]

### Bottleneck Identification
1. **Primary Bottleneck**: [Limiting factor]
   - Impact: [Effect on user experience]
   - Frequency: [How often this occurs]
   - Resolution: [Potential solutions]
```

### 4. Business Impact Assessment

**Business Metrics Analysis**:
```markdown
## Business Impact Analysis

### Key Performance Indicators
- **Revenue Impact**: [Financial effect of changes]
- **User Growth**: [Acquisition and retention trends]
- **Operational Efficiency**: [Process improvements]
- **Customer Satisfaction**: [Satisfaction scores and feedback]

### Feature Business Value
#### High-Value Features
- Feature A: [Usage: X%, Business Value: $Y]
- Feature B: [Usage: X%, Business Value: $Y]

#### Low-Value Features
- Feature C: [Usage: X%, Maintenance Cost: $Y]
- Feature D: [Usage: X%, Support Burden: Z hours]

### ROI Analysis
- **Development Investment**: [Time and resources spent]
- **Operational Costs**: [Ongoing maintenance and support]
- **Value Generated**: [Revenue, efficiency gains, cost savings]
- **Net Return**: [Overall return on investment]
```

## Insight Generation

### Actionable Insights Template
```markdown
## Insight: [Insight Title]

**Category**: [Performance/User Experience/Business/Quality]
**Priority**: [High/Medium/Low]
**Confidence**: [High/Medium/Low based on data quality]

### Finding
[What the data shows]

### Impact
- **User Impact**: [Effect on user experience]
- **Business Impact**: [Effect on business metrics]
- **Technical Impact**: [Effect on system performance]

### Recommendation
**Proposed Action**: [Specific recommendation]
**Expected Outcome**: [Predicted result]
**Implementation Effort**: [Time/resource estimate]
**Risk Assessment**: [Potential downsides]

### Success Metrics
[How to measure if the recommendation is successful]
```

## Outputs

### Primary Artifacts
- **Metrics Analysis Report** → `docs/helix/06-iterate/metrics-analysis/[date]-analysis.md`
- **Performance Dashboard** → `docs/helix/06-iterate/dashboards/performance-dashboard.md`
- **User Behavior Insights** → `docs/helix/06-iterate/insights/user-behavior-[date].md`

### Supporting Artifacts
- **Raw Data Exports** → `data/metrics/[date]/`
- **Analysis Notebooks** → `analysis/[date]-metrics-analysis.ipynb`
- **Trend Charts and Visualizations** → `docs/helix/06-iterate/charts/`

## Quality Gates

**Analysis Completion Criteria**:
- [ ] All key metric categories analyzed
- [ ] Trends and patterns identified
- [ ] Root cause analysis completed for issues
- [ ] Business impact quantified
- [ ] Actionable insights generated
- [ ] Recommendations prioritized
- [ ] Success metrics defined for recommendations
- [ ] Stakeholder review completed
- [ ] Next actions planned

## Integration with Iterate Phase

This action supports the Iterate phase by:
- **Informing Improvements**: Data-driven insights guide next iteration
- **Validating Assumptions**: Metrics confirm or refute hypotheses
- **Measuring Success**: Quantifies impact of previous changes
- **Identifying Opportunities**: Reveals areas for optimization

## Analysis Tools and Techniques

### Statistical Analysis
- **Trend Analysis**: Identify patterns over time
- **Correlation Analysis**: Find relationships between metrics
- **Anomaly Detection**: Identify unusual patterns
- **Cohort Analysis**: Track user groups over time
- **A/B Testing Results**: Compare feature variants

### Visualization Techniques
- **Time Series Charts**: Show trends over time
- **Heatmaps**: Show intensity patterns
- **Funnel Charts**: Show conversion paths
- **Distribution Charts**: Show value spreads
- **Correlation Matrices**: Show relationships

## Common Analysis Pitfalls

❌ **Cherry-Picking Data**: Selecting only favorable metrics
❌ **Correlation vs. Causation**: Assuming correlation implies causation
❌ **Insufficient Sample Size**: Drawing conclusions from too little data
❌ **Ignoring Context**: Not considering external factors
❌ **Analysis Paralysis**: Over-analyzing instead of taking action
❌ **Vanity Metrics**: Focusing on impressive but meaningless numbers

## Success Criteria

This action succeeds when:
- ✅ Comprehensive analysis of all key metrics completed
- ✅ Clear trends and patterns identified
- ✅ Root causes understood for performance issues
- ✅ Business impact quantified and communicated
- ✅ Actionable insights generated with clear recommendations
- ✅ Priorities established for next iteration improvements
- ✅ Success metrics defined for proposed changes
- ✅ Stakeholders understand findings and agree on next steps

Remember: Metrics without action are just numbers. Focus on insights that drive meaningful improvements to user experience and business value.