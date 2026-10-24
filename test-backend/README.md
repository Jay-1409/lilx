# lilx test backend

Small, dependency-free Node.js server with ten read-only JSON APIs and an OpenAPI 3 spec. The data is fixed sample data for testing lilx, not live business data.

```sh
node test-backend/server.js
```

It listens on `127.0.0.1:9000` by default. Set `PORT` to use another port. The checked-in [OpenAPI spec](openapi.json) is served at `http://127.0.0.1:9000/openapi.json` and describes parameters and response fields for all ten operations.

| Endpoint | Operation ID | Purpose |
| --- | --- | --- |
| `GET /customers` | `listCustomers` | All customers |
| `GET /customers/{id}` | `getCustomer` | One customer |
| `GET /products` | `listProducts` | Products and stock |
| `GET /products/{id}` | `getProduct` | One product |
| `GET /orders` | `listOrders` | Orders; optional `status`, `customer_id` |
| `GET /orders/{id}` | `getOrder` | One order |
| `GET /tickets` | `listTickets` | Tickets; optional `status`, `customer_id` |
| `GET /tickets/{id}` | `getTicket` | One ticket |
| `GET /incidents` | `listIncidents` | Incidents; optional `status`, `service` |
| `GET /metrics/checkout` | `getCheckoutMetrics` | Order failure counts and rate |

For lilx, set `LILX_BACKEND_URL=http://127.0.0.1:9000`, `LILX_OPENAPI_URL=http://127.0.0.1:9000/openapi.json`, and `LILX_ALLOWED_OPERATIONS` to the comma-separated operation IDs you want to approve. Start this server before starting lilx. No backend authorization header is needed.

Run the fixture check with `node --test test-backend/server.test.js`.
