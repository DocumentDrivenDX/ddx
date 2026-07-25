---
ddx:
  id: FIXTURE-WAIVER-IMPLEMENTED
spec:verification-waiver: "REQ-001 evidence not yet landed; temporary exception"
---
# Fixture Implemented With Waiver

**Status:** Implemented

## Requirements

### REQ-001: Create resource

The system MUST create the resource.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-001 | TestMissing | cd cli && go test ./pkg -run TestMissing |
