#!/usr/bin/env bash
set -euo pipefail

if [[ "${HELIX_SKIP_TRIAGE:-0}" == "1" ]]; then
  exit 0
fi

bead_json="$(cat)"

type="$(jq -r '.issue_type // ""' <<<"$bead_json")"
labels_csv="$(jq -r '[.labels[]?] | join(",")' <<<"$bead_json")"
spec_id="$(jq -r '."spec-id" // ""' <<<"$bead_json")"
acceptance="$(jq -r '.acceptance // ""' <<<"$bead_json")"
deps_csv="$(jq -r '[.dependencies[]?] | join(",")' <<<"$bead_json")"

error=0

case ",${labels_csv}," in
  *,helix,*)
    ;;
  *)
    printf '[triage] error: missing required label "helix"\n' >&2
    error=1
    ;;
esac

case ",${labels_csv}," in
  *,phase:frame,*|*,phase:design,*|*,phase:test,*|*,phase:build,*|*,phase:deploy,*|*,phase:iterate,*|*,phase:review,*)
    ;;
  *)
    printf '[triage] error: missing phase label\n' >&2
    error=1
    ;;
esac

if [[ "$type" == "task" && -z "$spec_id" ]]; then
  printf '[triage] error: tasks require --spec-id\n' >&2
  error=1
fi

if [[ "$type" == "task" || "$type" == "epic" ]]; then
  if [[ -z "$acceptance" ]]; then
    printf '[triage] error: %s issues require --acceptance\n' "$type" >&2
    error=1
  fi
fi

if [[ -n "$deps_csv" ]]; then
  for dep in ${deps_csv//,/ }; do
    if ! ddx bead show "$dep" --json >/dev/null 2>&1; then
      printf '[triage] error: dependency %s does not exist\n' "$dep" >&2
      error=1
    fi
  done
fi

if (( error == 1 )); then
  exit 1
fi

case ",${labels_csv}," in
  *,kind:*)
    ;;
  *)
    printf '[triage] warning: consider adding a kind label\n' >&2
    ;;
esac

exit 0
