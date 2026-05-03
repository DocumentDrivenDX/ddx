case "$CHECK_NAME" in
  envfixture)
    printf '{"status":"pass","message":"BEAD=%s BASE=%s HEAD=%s ROOT=%s RUN=%s"}' "$BEAD_ID" "$DIFF_BASE" "$DIFF_HEAD" "$PROJECT_ROOT" "$RUN_ID" > "$EVIDENCE_DIR/$CHECK_NAME.json"
    ;;
  slow-*)
    sleep 0.4
    printf '{"status":"pass","message":"slow"}' > "$EVIDENCE_DIR/$CHECK_NAME.json"
    ;;
  exiter)
    exit 9
    ;;
  missing)
    exit 0
    ;;
  *)
    printf '{"status":"error","message":"unknown fixture case"}' > "$EVIDENCE_DIR/$CHECK_NAME.json"
    ;;
esac
