process.stdin.setEncoding('utf8');

let data = '';
process.stdin.on('data', (chunk) => {
  data += chunk;
});

process.stdin.on('end', () => {
  const payload = data.trim();
  if (!payload) {
    console.log('node-basic: hello');
    return;
  }
  console.log(`node-basic: ${payload}`);
});
