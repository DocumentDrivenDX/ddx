const { processStrings } = require('./src/process.js');

// Generate test data: 5000 strings of 100 chars each, ~10% duplicates
const data = [];
const chars = 'abcdefghijklmnopqrstuvwxyz';
for (let i = 0; i < 5000; i++) {
  let s = '';
  const len = 50 + Math.floor(Math.random() * 50);
  for (let j = 0; j < len; j++) {
    s += chars[Math.floor(Math.random() * chars.length)];
  }
  // ~10% duplicates
  if (i > 0 && Math.random() < 0.1) {
    data.push(data[Math.floor(Math.random() * i)]);
  } else {
    data.push(s);
  }
}

const start = performance.now();
const result = processStrings(data);
const elapsed = performance.now() - start;

console.log(`Processed ${data.length} items → ${result.length} unique results`);
console.log(`METRIC runtime=${elapsed.toFixed(1)}`);
