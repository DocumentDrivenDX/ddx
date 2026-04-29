const { describe, it } = require('node:test');
const assert = require('node:assert');
const { toFahrenheit, toCelsius, toKelvin, fromKelvin } = require('../bin/convert.js');

describe('temperature conversion', () => {
  it('converts 0°C to 32°F', () => assert.strictEqual(toFahrenheit(0), 32));
  it('converts 212°F to 100°C', () => assert.strictEqual(toCelsius(212), 100));
  it('converts 98.6°F to 37°C', () => assert.strictEqual(Math.round(toCelsius(98.6) * 10) / 10, 37));
});

describe('kelvin conversion', () => {
  it('converts 0°C to 273.15K', () => assert.strictEqual(toKelvin(0), 273.15));
  it('converts 100°C to 373.15K', () => assert.strictEqual(toKelvin(100), 373.15));
  it('converts 273.15K to 0°C', () => assert.strictEqual(fromKelvin(273.15), 0));
  it('converts 0K to -273.15°C', () => assert.strictEqual(fromKelvin(0), -273.15));
});
