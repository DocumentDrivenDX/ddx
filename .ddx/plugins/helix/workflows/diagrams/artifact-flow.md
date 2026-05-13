# HELIX Artifact Flow and Dependencies

## Artifact Flow Between Phases

```mermaid
graph LR
    subgraph "📋 FRAME Artifacts"
        PRD[Product Requirements<br/>Document]
        US[User Stories]
        PS[Principles]
        FS[Feature Specs]
        FR[Feature Registry]
        SM[Stakeholder Map]
        RR[Risk Register]
        SR[Security Requirements]
        TM[Threat Model]
        CR[Compliance Requirements]
    end

    subgraph "🏗️ DESIGN Artifacts"
        SD[Solution Design]
        AD[Architecture Diagrams]
        API[API Contracts]
        ADR[Architecture Decisions]
        DD[Data Design]
        SA[Security Architecture]
        AUTH[Auth Design]
    end

    subgraph "🧪 TEST Artifacts"
        TP[Test Plan]
        TPR[Test Procedures]
        CT[Contract Tests]
        IT[Integration Tests]
        UT[Unit Tests]
        ST[Security Tests]
        PT[Performance Tests]
        TC[Test Coverage Report]
    end

    subgraph "⚙️ BUILD Artifacts"
        IP[Implementation Plan]
        BB[Build Issues]
        SC[Source Code]
        CD[Code Documentation]
        CR2[Code Reviews]
        BD[Build Documentation]
        DG[Deployment Guide]
    end

    subgraph "🚀 DEPLOY Artifacts"
        DB[Deploy Issues]
        RN[Release Notes]
        MC[Monitoring Config]
        RP[Runbook Procedures]
        IRP[Incident Response Plan]
    end

    subgraph "🔄 ITERATE Artifacts"
        MR[Metrics Report]
        RCA[Root Cause Analysis]
        LR[Lessons Report]
        IS[Improvement Suggestions]
        BI[Backlog Issues]
    end

    PRD --> SD
    US --> SD
    PS --> ADR
    FS --> API
    SR --> SA
    TM --> SA
    CR --> AUTH

    SD --> AD
    AD --> TP
    API --> CT
    ADR --> DD
    SA --> ST
    AUTH --> IT

    TP --> IP
    IP --> BB
    BB --> SC
    CT --> SC
    IT --> SC
    UT --> SC
    ST --> SC

    BB --> DB
    SC --> BD
    CD --> RN
    DG --> MC

    MC --> MR
    IRP --> RCA
    MR --> IS
    RCA --> LR
    IS --> BI

    BI -.->|Next Iteration| PRD

    style PRD fill:#e3f2fd
    style SD fill:#f3e5f5
    style TP fill:#ffebee
    style SC fill:#e8f5e9
    style DP fill:#fff3e0
    style MR fill:#fce4ec
```

## Artifact Dependency Matrix

```mermaid
graph TD
    subgraph "Critical Path Dependencies"
        PRD[PRD] --> US[User Stories]
        PRD --> FS[Feature Specs]
        US --> AC[Acceptance Criteria]
        FS --> TC[Test Cases]

        SD[Solution Design] --> AD[Architecture]
        AD --> API[API Contracts]
        API --> CT[Contract Tests]

        SR[Security Req] --> TM[Threat Model]
        TM --> SA[Security Arch]
        SA --> ST[Security Tests]

        TC --> UT[Unit Tests]
        CT --> IT[Integration Tests]
        ST --> PT[Penetration Tests]

        AllTests{All Tests<br/>Passing?}
        UT --> AllTests
        IT --> AllTests
        CT --> AllTests
        ST --> AllTests

        AllTests -->|Yes| Deploy[Deploy]
        AllTests -->|No| Fix[Fix Code]
        Fix --> AllTests
    end

    style PRD fill:#bbdefb
    style SD fill:#d1c4e9
    style SR fill:#ffcdd2
    style AllTests fill:#fff9c4,stroke:#f57c00,stroke-width:3px
    style Deploy fill:#c8e6c9
```

## Cross-Phase Traceability

```mermaid
graph TB
    subgraph "Requirements to Deployment Traceability"
        R1[Requirement<br/>REQ-001] --> US1[User Story<br/>US-001]
        US1 --> F1[Feature<br/>FEAT-001]
        F1 --> API1[API Contract<br/>API-001]
        API1 --> T1[Test Suite<br/>TEST-001]
        T1 --> C1[Code Module<br/>MOD-001]
        C1 --> D1[Deployment<br/>DEP-001]
        D1 --> M1[Metric<br/>MET-001]
        M1 -.->|Validates| R1
    end

    style R1 fill:#e1f5fe
    style US1 fill:#f3e5f5
    style F1 fill:#fff3e0
    style API1 fill:#e8f5e9
    style T1 fill:#ffebee
    style C1 fill:#e0f2f1
    style D1 fill:#fce4ec
    style M1 fill:#f1f8e9
```

## Artifact Creation Timeline

```mermaid
gantt
    title HELIX Artifact Creation Timeline
    dateFormat YYYY-MM-DD
    section FRAME
    PRD                 :done, frame1, 2025-01-01, 2d
    User Stories        :done, frame2, after frame1, 2d
    Feature Specs       :done, frame3, after frame2, 2d
    Security Requirements :done, frame4, 2025-01-01, 3d
    Threat Model        :done, frame5, after frame4, 2d

    section DESIGN
    Solution Design     :active, design1, after frame3, 3d
    Architecture        :active, design2, after design1, 2d
    API Contracts       :active, design3, after design2, 2d
    Security Architecture :active, design4, after frame5, 3d

    section TEST
    Test Plan          :crit, test1, after design3, 2d
    Contract Tests     :crit, test2, after test1, 3d
    Integration Tests  :crit, test3, after test2, 2d
    Security Tests     :crit, test4, after design4, 2d

    section BUILD
    Implementation     :build1, after test4, 5d
    Code Review        :build2, after build1, 2d
    Documentation      :build3, after build2, 1d

    section DEPLOY
    Deployment Prep    :deploy1, after build3, 1d
    Release           :deploy2, after deploy1, 1d
    Monitoring        :deploy3, after deploy2, 1d

    section ITERATE
    Metrics Analysis   :iterate1, after deploy3, 2d
    Improvements      :iterate2, after iterate1, 2d
```

## Security Artifact Dependencies

```mermaid
graph TD
    subgraph "Security Artifact Chain"
        CR[Compliance<br/>Requirements] --> SR[Security<br/>Requirements]
        SR --> TM[Threat Model<br/>STRIDE Analysis]
        TM --> RA[Risk<br/>Assessment]
        RA --> SA[Security<br/>Architecture]
        SA --> SC[Security<br/>Controls]
        SC --> ST[Security<br/>Tests]
        ST --> SM[Security<br/>Monitoring]
        SM --> IR[Incident<br/>Response]
        IR --> SI[Security<br/>Improvements]
        SI -.->|Feedback| SR
    end

    style CR fill:#ffebee
    style SR fill:#ffcdd2
    style TM fill:#ef9a9a
    style RA fill:#e57373
    style SA fill:#ef5350
    style SC fill:#f44336
    style ST fill:#e53935
    style SM fill:#d32f2f
    style IR fill:#c62828
    style SI fill:#b71c1c
```

## Artifact Quality Gates

```mermaid
graph LR
    subgraph "Artifact Validation Flow"
        Create[Create<br/>Artifact] --> Review{Peer<br/>Review}
        Review -->|Feedback| Revise[Revise]
        Revise --> Review
        Review -->|Approved| Validate{Automated<br/>Validation}
        Validate -->|Failed| Fix[Fix Issues]
        Fix --> Validate
        Validate -->|Passed| Sign{Stakeholder<br/>Sign-off}
        Sign -->|Changes| Revise
        Sign -->|Approved| Complete[✓ Complete]
    end

    style Create fill:#e3f2fd
    style Review fill:#fff9c4
    style Validate fill:#fff9c4
    style Sign fill:#fff9c4
    style Complete fill:#c8e6c9
    style Revise fill:#ffccbc
    style Fix fill:#ffccbc
```

## Key Artifact Relationships

### Critical Dependencies
1. **PRD → All Design Artifacts**: Requirements drive all technical decisions
2. **API Contracts → Contract Tests**: Every API must have corresponding tests
3. **Threat Model → Security Tests**: Each threat must have validation tests
4. **Test Plan → Build Issues**: Tests define the work that implementation issues must satisfy
5. **Metrics → Next Iteration PRD**: Production data informs future requirements

### Validation Chains
- Requirements → Stories → Tests → Code → Deployment → Metrics
- Security Requirements → Architecture → Controls → Tests → Monitoring
- Principles → Decisions → Implementation → Validation → Compliance

### Feedback Loops
- Metrics feed back to Requirements
- Incidents feed back to Security Requirements
- Iteration learnings feed back to Principles
- Test Results feed back to Design

## Artifact Storage Structure

```
docs/
├── 01-frame/
│   ├── prd.md
│   ├── principles.md
│   ├── user-stories/
│   ├── features/
│   ├── security-requirements.md
│   └── threat-model.md
├── 02-design/
│   ├── architecture.md
│   ├── solution-designs/
│   ├── contracts/
│   ├── adr/
│   └── security-architecture.md
├── 03-test/
│   ├── test-plan.md
│   ├── test-procedures.md
│   └── test-suites/
├── 04-build/
│   └── implementation-plan.md
├── 05-deploy/
│   ├── deployment-checklist.md
│   ├── monitoring-setup.md
│   ├── runbook.md
│   └── release-notes.md
└── 06-iterate/
    ├── metrics-dashboard.md
    ├── security-metrics.md
    └── improvement-backlog.md
```
