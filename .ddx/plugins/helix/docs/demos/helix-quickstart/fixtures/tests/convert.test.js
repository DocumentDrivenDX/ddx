const test = require('node:test');
const assert = require('node:assert');
const { toFahrenheit, toCelsius } = require('../bin/convert.js');

test('toFahrenheit(0) === 32', () => {
  assert.strictEqual(toFahrenheit(0), 32);
});

test('toCelsius(212) === 100', () => {
  assert.strictEqual(toCelsius(212), 100);
});

test('toCelsius(98.6) is approximately 37.0', () => {
  const result = toCelsius(98.6);
  assert.ok(Math.abs(result - 37.0) < 0.05, `Expected ~37.0 but got ${result}`);
});
