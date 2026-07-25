---
ddx:
  id: FIXTURE-WAIVER-COMPLETE
spec:verification-waiver: "REQ-001 evidence not yet landed; temporary exception"
---
# Fixture Complete With Waiver

**Status:** Complete

## Requirements

### REQ-001: Create resource

The system MUST create the resource.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-001 | TestMissing | cd cli && go test ./pkg -run TestMissing |
