import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createServer } from 'node:http';
import { once } from 'node:events';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

function run(url) {
  const child = spawn(process.execPath, [fileURLToPath(new URL('./wait-for-frontend.mjs', import.meta.url))], {
    env: { FRONTEND_DEVSERVER_URL: url }, stdio: ['ignore', 'pipe', 'pipe']
  });
  let stdout = ''; let stderr = '';
  child.stdout.on('data', data => { stdout += data; });
  child.stderr.on('data', data => { stderr += data; });
  return { child, result: once(child, 'close').then(([code]) => ({ code, stdout, stderr })) };
}

test('waits through cold frontend responses using the IPv4 localhost proxy target', { timeout: 10000 }, async () => {
  let requests = 0;
  const server = createServer((request, response) => { response.writeHead(++requests < 3 ? 503 : 200); response.end('ready'); });
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  const { child, result } = run(`http://localhost:${server.address().port}`);
  try {
    const output = await result;
    assert.equal(output.code, 0, output.stderr);
    assert.ok(requests >= 3);
    assert.match(output.stdout, /frontend is ready/);
  } finally { child.kill(); server.closeAllConnections(); server.close(); }
});

test('rejects a non-loopback frontend before making a connection', { timeout: 5000 }, async () => {
  const { child, result } = run('http://example.com');
  try { const output = await result; assert.equal(output.code, 1); assert.match(output.stderr, /loopback/); }
  finally { child.kill(); }
});
