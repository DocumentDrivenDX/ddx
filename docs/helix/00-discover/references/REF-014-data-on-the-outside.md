---
ddx:
  id: REF-014
  status: published
id: REF-014
title: "Data on the Outside vs. Data on the Inside"
kind: reference
source_url: https://queue.acm.org/detail.cfm?id=3415014
source_author: Pat Helland
source_organization: ACM Queue
accessed: 2026-05-01
summary: "Distinguishes immutable, versioned data flowing between services ('outside') from mutable internal state ('inside'), arguing the outside is the contract that survives change."
tags: [data, contracts, distributed-systems, foundational]
---

# Helland — Data on the Outside vs. Data on the Inside

Foundational essay framing inter-service data as immutable, versioned facts that act as contracts. Cited to motivate DDx's treatment of governing artifacts (specs, beads, ADRs) as the durable "outside" data that agents and humans both consume.
