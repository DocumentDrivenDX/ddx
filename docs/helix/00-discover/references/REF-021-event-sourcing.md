---
ddx:
  id: REF-021
  status: published
id: REF-021
title: "Event Sourcing"
kind: reference
source_url: https://martinfowler.com/eaaDev/EventSourcing.html
source_author: Martin Fowler
accessed: 2026-05-01
summary: "Architectural pattern that records all state changes as an immutable, ordered event log; current state is a derived projection rebuildable at any time."
tags: [architecture, event-sourcing, foundational]
---

# Fowler — Event Sourcing

Reference for treating state as a replayable log of immutable events. Backs DDx's bead-update model and execution-evidence trail: the log is the truth; everything else is a projection.
