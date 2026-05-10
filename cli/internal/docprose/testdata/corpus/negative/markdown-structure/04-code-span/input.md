Use `ddx bead create` to file a new work item.
The `Checker.Findings` method accepts a file path and text, returning a slice of `Finding` values.
Call `NewChecker(mode, vocabulary)` to initialize the checker with the desired mode.
Each `Finding` has fields: `File`, `Line`, `RuleID`, `Severity`, `Rationale`, `SuggestedEdit`.
