# Fixture With Malformed Verification Rows

**Status:** Complete

## Requirements

### REQ-100: Happy path

A well-formed requirement.

### REQ-200: Needs coverage

Another requirement.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-100 | TestHappyPath | cd cli && go test ./pkg -run TestHappyPath |
|  | TestMissingReq | cd cli && go test ./pkg -run TestMissingReq |
| REQ-200 |  | cd cli && go test ./pkg -run TestSomething |
| REQ-200 | TestMissingCmd |  |
