# Execution Evidence: ddx-a41e6fbb

## AC Verification

1. **TestPreClaimIntakeRewrite_AllowsCompressionReplacement** — passes in `cli/internal/agent/preclaim_intake_rewrite_test.go`
2. **TestPreClaimIntakeRewrite_AllowsExpansionReplacement** — passes
3. **TestPreClaimIntakeRewrite_RejectsDeletedCommitment** — passes (4 subtests: governing_ref_dropped, non_scope_bullet_dropped, named_test_dropped, file_line_dropped)
4. **TestPreClaimIntakeRewrite_RecordsReplacementEvidence** — passes; event body verified to contain `preservation_evidence` field
5. **TestPreClaimIntakePrompt_AsksForFitForPurposeValidatedReplacement** — passes; prompt checked for "prompt fitness", "replacement", "ambiguous_needs_human"
6. **TestIntake_ActionableButRewritten_UpdatesAfterClaim** — green
7. **TestIntake_UnsafeRewriteBlocksForHuman** — green
8. `cd cli && go test ./internal/agent/... -run "TestPreClaimIntakeRewrite|TestIntake_.*Rewrite" -count=1` — passes
