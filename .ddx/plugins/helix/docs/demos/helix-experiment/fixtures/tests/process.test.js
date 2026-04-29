const { describe, it } = require('node:test');
const assert = require('node:assert');
const { processStrings } = require('../src/process.js');

describe('processStrings', () => {
  it('uppercases all items', () => {
    const result = processStrings(['hello', 'world']);
    assert.deepStrictEqual(result, ['HELLO', 'WORLD']);
  });

  it('deduplicates results', () => {
    const result = processStrings(['hello', 'hello', 'world']);
    assert.deepStrictEqual(result, ['HELLO', 'WORLD']);
  });

  it('handles empty input', () => {
    const result = processStrings([]);
    assert.deepStrictEqual(result, []);
  });

  it('handles single character strings', () => {
    const result = processStrings(['a', 'b', 'a']);
    assert.deepStrictEqual(result, ['A', 'B']);
  });
});
