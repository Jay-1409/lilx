const http = require('node:http');
const spec = require('./openapi.json');

const customers = [
  { id: 'cus_1', name: 'Northstar Books', tier: 'enterprise' },
  { id: 'cus_2', name: 'Cedar Market', tier: 'standard' },
];
const products = [
  { id: 'prod_1', name: 'Notebook', price_cents: 1200, stock: 34 },
  { id: 'prod_2', name: 'Desk Lamp', price_cents: 4500, stock: 0 },
];
const orders = [
  { id: 'ord_101', customer_id: 'cus_1', product_id: 'prod_1', status: 'failed', total_cents: 2400, error: 'checkout_timeout' },
  { id: 'ord_102', customer_id: 'cus_1', product_id: 'prod_2', status: 'paid', total_cents: 4500, error: null },
  { id: 'ord_103', customer_id: 'cus_2', product_id: 'prod_1', status: 'paid', total_cents: 1200, error: null },
  { id: 'ord_104', customer_id: 'cus_2', product_id: 'prod_1', status: 'failed', total_cents: 1200, error: 'checkout_timeout' },
];
const tickets = [
  { id: 'tick_1', customer_id: 'cus_1', status: 'open', subject: 'Checkout times out', order_id: 'ord_101', priority: 'high' },
  { id: 'tick_2', customer_id: 'cus_2', status: 'open', subject: 'Cannot finish checkout', order_id: 'ord_104', priority: 'medium' },
  { id: 'tick_3', customer_id: 'cus_1', status: 'closed', subject: 'Invoice copy', order_id: 'ord_102', priority: 'low' },
];
const incidents = [
  { id: 'inc_1', status: 'active', service: 'checkout', title: 'Intermittent checkout timeouts', started_at: '2026-10-24T08:00:00Z' },
  { id: 'inc_2', status: 'resolved', service: 'catalog', title: 'Catalog sync delay', started_at: '2026-10-20T09:00:00Z' },
];

const routes = [
  { path: '/customers', id: 'listCustomers', summary: 'List customers', get: () => customers },
  { path: '/customers/{id}', id: 'getCustomer', summary: 'Get one customer by ID', get: (_, id) => customers.find(x => x.id === id) },
  { path: '/products', id: 'listProducts', summary: 'List products and stock', get: () => products },
  { path: '/products/{id}', id: 'getProduct', summary: 'Get one product by ID', get: (_, id) => products.find(x => x.id === id) },
  { path: '/orders', id: 'listOrders', summary: 'List orders; filter by status or customer_id', query: ['status', 'customer_id'], get: q => orders.filter(x => (!q.status || x.status === q.status) && (!q.customer_id || x.customer_id === q.customer_id)) },
  { path: '/orders/{id}', id: 'getOrder', summary: 'Get one order by ID', get: (_, id) => orders.find(x => x.id === id) },
  { path: '/tickets', id: 'listTickets', summary: 'List support tickets; filter by status or customer_id', query: ['status', 'customer_id'], get: q => tickets.filter(x => (!q.status || x.status === q.status) && (!q.customer_id || x.customer_id === q.customer_id)) },
  { path: '/tickets/{id}', id: 'getTicket', summary: 'Get one support ticket by ID', get: (_, id) => tickets.find(x => x.id === id) },
  { path: '/incidents', id: 'listIncidents', summary: 'List incidents; filter by status or service', query: ['status', 'service'], get: q => incidents.filter(x => (!q.status || x.status === q.status) && (!q.service || x.service === q.service)) },
  { path: '/metrics/checkout', id: 'getCheckoutMetrics', summary: 'Get checkout order counts and error rate', get: () => {
    const failed = orders.filter(x => x.status === 'failed').length;
    return { total_orders: orders.length, failed_orders: failed, order_error_rate_percent: failed / orders.length * 100 };
  } },
];

function createServer() {
  return http.createServer((req, res) => {
    const send = (status, data) => {
      res.writeHead(status, { 'content-type': 'application/json; charset=utf-8' });
      res.end(JSON.stringify(data));
    };
    if (req.method !== 'GET') return send(405, { error: 'Only GET is supported' });
    const url = new URL(req.url, 'http://localhost');
    if (url.pathname === '/openapi.json') return send(200, spec);
    for (const route of routes) {
      const match = url.pathname.match(new RegExp('^' + route.path.replace('{id}', '([^/]+)') + '$'));
      if (!match) continue;
      const query = Object.fromEntries(url.searchParams);
      if (Object.keys(query).some(key => !(route.query || []).includes(key))) return send(400, { error: 'Unknown query parameter' });
      const data = route.get(query, match[1]);
      return data === undefined ? send(404, { error: 'Not found' }) : send(200, data);
    }
    return send(404, { error: 'Not found' });
  });
}

if (require.main === module) {
  const port = Number(process.env.PORT || 9000);
  createServer().listen(port, '127.0.0.1', () => console.log(`Test backend: http://127.0.0.1:${port}`));
}

module.exports = { createServer };
