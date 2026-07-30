import { createServer } from 'http';

function greet(name) {
  if (!name) {
    return 'Hello, World!';
  }
  if (name.toLowerCase() === 'coverport') {
    return 'Hello from the CoverPort test fixture!';
  }
  return `Hello, ${name}!`;
}

const server = createServer((req, res) => {
  const url = new URL(req.url, `http://localhost`);

  if (url.pathname === '/hello') {
    const name = url.searchParams.get('name') || '';
    const msg = greet(name);
    res.writeHead(200, { 'Content-Type': 'text/plain' });
    res.end(msg + '\n');
  } else {
    res.writeHead(404);
    res.end();
  }
});

const port = process.env.APP_PORT || 8080;
server.listen(port, '0.0.0.0', () => {
  console.log(`Test fixture app listening on :${port}`);
});
