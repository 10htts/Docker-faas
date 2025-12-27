process.stdin.setEncoding('utf8');

let data = '';
process.stdin.on('data', (chunk) => {
  data += chunk;
});

process.stdin.on('end', () => {
  const payload = data.trim();
  if (!payload) {
    console.log('Hello from docker-faas (node). No input provided.');
    return;
  }
  console.log(`Hello from docker-faas (node). Input: ${payload}`);
});
