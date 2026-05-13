---
ddx:
  id: "[artifact-id]"
---

# Threat Model

**Project**: [Project Name]
**Date**: [Creation Date]

## Executive Summary

**System Overview**: [Brief description]
**Key Assets**: [Primary assets needing protection]
**Primary Threats**: [Top 3-5 threats]
**Risk Level**: [Critical/High/Medium/Low]

## System Description

### Boundaries and Components
**In Scope**: [Systems, components, data flows included]
**Out of Scope**: [What is not covered]
**Trust Boundaries**: [Where trust levels change]

### Components
| Component | Description | Trust Level |
|-----------|-------------|-------------|
| [Component] | [Description] | [Level] |

### Data Flows
- **External Sources**: [Data entering the system]
- **Internal Processing**: [How data moves within]
- **External Destinations**: [Where data exits]

## Assets

### Data Assets
| Asset | Classification | Confidentiality | Integrity | Availability |
|-------|---------------|-----------------|-----------|--------------|
| [Asset] | [Level] | [Criticality] | [Criticality] | [Criticality] |

### System Assets
| Asset | Criticality | Dependencies |
|-------|-------------|--------------|
| [Asset] | [Level] | [Dependencies] |

## STRIDE Threat Analysis

For each STRIDE category (Spoofing, Tampering, Repudiation, Information Disclosure, Denial of Service, Elevation of Privilege):

| ID | Threat | Impact | Likelihood | Risk | Mitigation |
|----|--------|--------|------------|------|------------|
| TM-X-001 | [Threat] | [Level] | [Level] | [Level] | [Control] |

ID prefix: S=Spoofing, T=Tampering, R=Repudiation, I=Information Disclosure, D=Denial of Service, E=Elevation of Privilege.

## Risk Assessment

**Scoring**: Impact (1-5) x Likelihood (1-5)
- **Critical (20-25)**: Immediate action required
- **High (15-19)**: Action within 30 days
- **Medium (10-14)**: Action within 90 days
- **Low (1-9)**: Monitor or accept

### Top Risks
| Risk ID | Threat | Impact | Likelihood | Score | Priority |
|---------|--------|--------|------------|-------|----------|
| [ID] | [Threat] | [1-5] | [1-5] | [Score] | [Level] |

## Mitigation Strategies

### [Risk ID] - [Title]
- **Controls**: [Preventive, detective, corrective actions]
- **Timeline**: [When to implement]
- **Owner**: [Who is responsible]
- **Verification**: [Security test, design review, or audit evidence]

## Security Controls Summary

- **Preventive**: [Authentication, authorization, encryption, input validation]
- **Detective**: [Logging, monitoring, intrusion detection]
- **Corrective**: [Incident response, backup/recovery, patching]

## Assumptions and Dependencies
- [List assumptions and external dependencies]
