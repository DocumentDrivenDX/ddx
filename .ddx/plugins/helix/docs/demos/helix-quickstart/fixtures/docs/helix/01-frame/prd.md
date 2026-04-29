# Product Requirements Document: hello-helix

## Overview
A Node.js CLI tool for converting temperatures between Celsius and Fahrenheit.

## Features

### Core Commands
1. **convert --to-celsius `<temperature>`**
   - Converts Fahrenheit to Celsius
   - Input: numeric temperature value
   - Output: result formatted to one decimal place

2. **convert --to-fahrenheit `<temperature>`**
   - Converts Celsius to Fahrenheit
   - Input: numeric temperature value
   - Output: result formatted to one decimal place

## Output Format
All results printed to stdout with one decimal place precision (e.g., `100.0`, `32.0`).

## Priority Levels

### P0: Core Functionality
- [ ] Implement `convert --to-celsius` command
- [ ] Implement `convert --to-fahrenheit` command
- [ ] Format output to one decimal place

### P1: Reliability & UX
- [ ] Validate numeric input
- [ ] Error messages for invalid/missing arguments
- [ ] Exit codes (0 for success, 1 for error)

### P2: Future Enhancement
- [ ] Batch mode: read from stdin, convert multiple values
- [ ] Support additional units (Kelvin, Rankine)
- [ ] Configuration file for defaults

## Non-Goals
- GUI or web interface
- Real-time weather integration
- Offline documentation beyond --help
