# Fixture Without Requirement IDs

**Status:** Implemented

## Overview

This document has no REQ-* identifiers.

## Architecture

Stable section anchors form the inventory when no requirement IDs exist.

## Data Model

Describes stored shapes.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| overview | TestOverview | cd cli && go test ./pkg -run TestOverview |
| architecture | TestArchitecture | cd cli && go test ./pkg -run TestArchitecture |
| data-model | .ddx/executions/fixture/report.json | test -f .ddx/executions/fixture/report.json |
