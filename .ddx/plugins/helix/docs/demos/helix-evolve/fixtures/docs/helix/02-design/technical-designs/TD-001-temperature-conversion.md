# TD-001: Temperature Conversion

## Architecture
- Single file: bin/convert.js
- Exports: toFahrenheit(c), toCelsius(f), toKelvin(c), fromKelvin(k)
- CLI: parse process.argv for flags (--to-celsius, --to-fahrenheit, --to-kelvin, --from-kelvin)
- Formulas: F = C * 9/5 + 32, C = (F - 32) * 5/9, K = C + 273.15
