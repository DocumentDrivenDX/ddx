---
ddx:
  id: FIXTURE-WAIVER-IN-PROGRESS
spec:verification-waiver: "REQ-001 evidence not yet landed; temporary exception"
---
# Fixture In Progress With Waiver

**Status:** In Progress

## Requirements

### REQ-001: Create resource

The system MUST create the resource.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-001 | TestMissing | cd cli && go test ./pkg -run TestMissing |
