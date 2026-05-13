---
ddx:
  id: "[artifact-id]"
---

# Metric Definition: [NAME]

> Store at `docs/helix/06-iterate/metrics/[NAME].yaml`

```yaml
name: [kebab-case-identifier]
description: [What this metric measures]
unit: [seconds|bytes|percent|count|...]
direction: [lower|higher]
command: [repeatable shell command]
output_pattern: "[regex with capture group]"
tolerance: [noise band, e.g. "5%" or "100ms"]
interpretation: [How to read meaningful changes]
labels:
  [key]: [value]
```
