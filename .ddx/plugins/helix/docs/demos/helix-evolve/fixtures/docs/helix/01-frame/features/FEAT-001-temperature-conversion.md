# FEAT-001: Temperature Conversion

## Functional Requirements
- FR-1: toFahrenheit(celsius) returns fahrenheit
- FR-2: toCelsius(fahrenheit) returns celsius
- FR-3: CLI accepts --to-celsius and --to-fahrenheit flags
- FR-4: Output rounded to one decimal place
- FR-5: toKelvin(celsius) returns kelvin
- FR-6: fromKelvin(kelvin) returns celsius

## Acceptance Criteria
- convert --to-celsius 212 → 100.0
- convert --to-fahrenheit 0 → 32.0
- convert --to-celsius 98.6 → 37.0
- convert --to-kelvin 100 → 373.1
- convert --from-kelvin 273.15 → 0.0
