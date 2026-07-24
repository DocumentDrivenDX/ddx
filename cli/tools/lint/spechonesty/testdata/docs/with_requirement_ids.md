---
ddx:
  id: FIXTURE-WITH-REQS
---
# Fixture With Requirement IDs

**Status:** Complete

## Requirements

### REQ-001: Create resource

The system MUST create the resource.

### REQ-002: List resources

The system MUST list resources.

### REQ-010: Delete resource

The system MUST delete the resource.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-001 | TestCreateResource | cd cli && go test ./pkg -run TestCreateResource |
| REQ-002 | TestListResources | cd cli && go test ./pkg -run TestListResources |
| REQ-010 | check:static-delete | go run ./tools/lint/deletecheck |
