// Deliberately slow: rebuilds the string character by character
function processStrings(items) {
  const seen = new Set();
  const results = [];
  for (let i = 0; i < items.length; i++) {
    const item = items[i];
    const result = item.toUpperCase();
    // Use Set for O(1) duplicate check instead of indexOf
    if (!seen.has(result)) {
      seen.add(result);
      results.push(result);
    }
  }
  return results;
}

module.exports = { processStrings };
