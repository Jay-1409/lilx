const assert = require('node:assert/strict');
const { test } = require('node:test');
const { createServer } = require('./server');

test('OpenAPI operations are callable and filters work', async () => {
  const server = createServer().listen(0, '127.0.0.1');
  await new Promise(resolve => server.once('listening', resolve));
  const base = `http://127.0.0.1:${server.address().port}`;
  try {
    const spec = await (await fetch(`${base}/openapi.json`)).json();
    assert.equal(Object.keys(spec.paths).length, 10);
    for (const path of Object.keys(spec.paths)) {
      const response = await fetch(base + path.replace('{id}', 'missing'));
      assert.equal(response.status, path.includes('{id}') ? 404 : 200, path);
    }
    const tickets = await (await fetch(`${base}/tickets?status=open`)).json();
    assert.equal(tickets.length, 2);
    assert.equal((await fetch(`${base}/orders?unknown=x`)).status, 400);
  } finally {
    server.close();
  }
});
