# Feature: FEAT-001 — Temperature Conversion

## Description
Enable users to convert temperatures between Celsius and Fahrenheit via command-line commands.

## User Stories

### Story 1: Convert Fahrenheit to Celsius
**As a** developer  
**I want to** convert Fahrenheit to Celsius  
**So that** I can understand temperature readings in metric units

#### Acceptance Criteria
- **Given** the command `convert --to-celsius 212`
- **When** executed
- **Then** output is `100.0`

- **Given** the command `convert --to-celsius 98.6`
- **When** executed
- **Then** output is `37.0`

### Story 2: Convert Celsius to Fahrenheit
**As a** developer  
**I want to** convert Celsius to Fahrenheit  
**So that** I can communicate temperature in imperial units

#### Acceptance Criteria
- **Given** the command `convert --to-fahrenheit 0`
- **When** executed
- **Then** output is `32.0`

- **Given** the command `convert --to-fahrenheit 37`
- **When** executed
- **Then** output is `98.6`

## Conversion Formulas
- °C = (°F − 32) × 5/9
- °F = °C × 9/5 + 32

## Implementation Notes
- Output always formatted to exactly one decimal place
- Accept integer and float inputs
- Rounding: standard floating-point rounding
