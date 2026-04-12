---
ddx:
  id: TP-006
  depends_on:
    - FEAT-006
    - SD-006
    - TD-006
---
# Test Plan: Agent Session Capture and Inspection

## Scope

Validate full-body agent session capture, backward-compatible reading, and
read-only inspection from the CLI and server surfaces.

## Test Cases

### Capture

- `TestAgentRunWritesPromptAndResponseToSessionRecord`
- `TestAgentRunWritesCorrelationMetadata`
- `TestAgentRunRecordsTokensDurationAndExitCode`

### Inspection

- `TestAgentLogListsRecentSessions`
- `TestAgentLogShowsFullSessionDetail`
- `TestAgentLogDetailIncludesPromptAndResponseBodies`

### Compatibility

- `TestAgentLogReadsMetadataOnlyLegacyRows`
- `TestAgentLogSkipsUnknownFieldsWithoutLoss`

### Redaction and Retention

- `TestAgentSessionRedactionMasksConfiguredPatterns`
- `TestAgentSessionLogDirConfigControlsOutputPath`

### Concurrency and Atomicity

- `TestAgentSessionRecordWriteIsAtomicUnderConcurrentWriters`
- `TestAgentSessionReaderDoesNotObservePartialRecord`

### Server/API Parity

- `TestServerAgentSessionListMatchesCLI`
- `TestServerAgentSessionDetailMatchesCLI`

## Fixtures

- `cli/internal/agent/testdata/legacy-session.jsonl`
- `cli/internal/agent/testdata/agent-sessions.jsonl`
- `cli/internal/agent/testdata/agent-sessions.d/as-1a2b3c4d/prompt.txt`
- `cli/internal/agent/testdata/redacted-session/`

## Exit Criteria

- CLI and server can list and inspect sessions using the same underlying log
  format.
- Old metadata-only rows still load and list correctly.
- Full prompt and response capture is visible from the detail path.
- Concurrent writes do not corrupt the session store.
