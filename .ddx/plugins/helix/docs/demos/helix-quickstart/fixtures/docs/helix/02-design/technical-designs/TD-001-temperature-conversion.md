# TD-001: Temperature Conversion Implementation

## Overview
Technical design for the temperature conversion feature. Single-file implementation providing command-line temperature conversion between Celsius and Fahrenheit.

## Architecture
- **Single module**: `bin/convert.js`
- **No external dependencies** (core Node.js only)
- **Two pure functions** with no side effects
- **CLI wrapper** around conversion logic

## Functions

### toFahrenheit(celsius)
Converts Celsius to Fahrenheit.
```
formula: (celsius × 9/5) + 32
returns: number
```

### toCelsius(fahrenheit)
Converts Fahrenheit to Celsius.
```
formula: (fahrenheit - 32) × 5/9
returns: number
```

## CLI Interface
Parses `process.argv` with two flags:
- `--to-fahrenheit <value>`: Convert from Celsius to Fahrenheit
- `--to-celsius <value>`: Convert from Fahrenheit to Celsius

Only one flag allowed per invocation. Invalid input (non-numeric values, both flags, no flags) prints usage and exits 1.

## Output Format
- Decimal precision: **one decimal place** via `toFixed(1)`
- Console output: `<result>°F` or `<result>°C`

## Error Handling
Invalid invocations:
1. Missing required flag → print usage
2. Non-numeric input → print usage
3. Both flags provided → print usage
4. Exit code: 1 for all errors

Usage message:
```
Usage: convert.js --to-celsius <value> | --to-fahrenheit <value>
```

## Testing Contract
Tests verify:
- Correct conversion math
- Proper decimal formatting (toFixed(1))
- CLI flag parsing
- Error handling and exit codes
