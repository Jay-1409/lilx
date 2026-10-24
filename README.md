# lilx

lilx is an HTTP service that turns a natural-language data request into approved backend API calls. It reads an OpenAPI 3.x spec at startup, exposes the approved GET operations, asks Gemma 4 to choose calls, fetches their JSON responses, and returns data validated against the caller's JSON Schema.

For local integration testing, run the dependency-free [test backend](test-backend/README.md), which exposes ten sample APIs and an OpenAPI spec.

## Configure

Set these environment variables before starting the server. A local `.env` is provided; fill it in, then load it in your shell and run lilx:

```sh
set -a
source .env
set +a
go run .
```

| Variable | Purpose |
| --- | --- |
| `LILX_BACKEND_URL` | Backend base URL, such as `http://127.0.0.1:9000` |
| `LILX_OPENAPI_URL` | URL of the backend's OpenAPI 3.x JSON or YAML spec |
| `LILX_ALLOWED_OPERATIONS` | `*` for every supported GET operation, or comma-separated approved `operationId` values |
| `LILX_API_KEY` | Optional caller bearer token; blank disables lilx caller authentication |
| `GEMINI_API_KEY` | Server-side key for Google Generative Language |
| `LILX_BACKEND_AUTHORIZATION` | Optional complete `Authorization` header sent to the backend |
| `LILX_OPENAPI_AUTHORIZATION` | Optional complete `Authorization` header used to fetch the spec |
| `LILX_MODEL` | Optional: `gemma-4-26b-a4b-it` (default) or `gemma-4-31b-it` |
| `LILX_MODEL_TIMEOUT_SECONDS` | Timeout for each Gemma call (default `60`, range `1`–`300`); backend calls keep their 15-second timeout |
| `LILX_WORKERS` | Backend call workers shared across requests (default `10`, range `1`–`100`) |
| `LILX_LOG_DIR` | Directory for one JSONL conversation log per authorized request (default `logs`) |
| `LILX_LISTEN_ADDR` | Optional listen address (default `127.0.0.1:8080`) |

The spec URL and backend URL are operator-controlled startup settings. A caller cannot change them through a prompt. An OpenAPI GET operation without an `operationId` is named `GET /its/path` for allowlisting.

## Use

`GET /v1/operations` lists approved operations. `POST /v1/requests` accepts:

```json
{
  "nlp_data_required": "What is the status of order 123?",
  "response_format": {
    "type": "object",
    "properties": {
      "status": {"type": "string"}
    },
    "required": ["status"],
    "additionalProperties": false
  }
}
```

When `LILX_API_KEY` is set, send `Authorization: Bearer <LILX_API_KEY>` to both product endpoints. When it is blank, no caller token is required; keep the listener on `127.0.0.1` unless you intend to expose it. A successful response contains `data` in the requested shape and a separate `trace` with selected operation IDs and HTTP statuses. Invalid plans, upstream failures, and output that does not match the schema return errors.

## Test with Bruno

Open the [`bruno`](bruno) folder as a Bruno collection and select the `local` environment. Set the secret `lilxApiKey` in Bruno to the same value as the server's `LILX_API_KEY`, then run **Gemma test**. The request calls `POST /v1/gemma/test` with a short prompt; the response is `{"text":"..."}`. The server uses its configured Gemma model and `GEMINI_API_KEY`, so the Google key never enters the Bruno collection. This test endpoint requires the same bearer token as the other lilx endpoints.

To test the product flow, start the [test backend](test-backend/README.md), configure lilx to use its OpenAPI URL and approve `listTickets`, `getCheckoutMetrics`, and `listIncidents` (or `*`), then run **Checkout impact** in Bruno. It calls `POST /v1/requests`; lilx selects the relevant backend operations and returns the requested data plus a call trace. The backend values are fixed samples for local testing.

The current execution boundary supports inline path and query parameters on approved GET operations, at most ten calls per request, and JSON backend responses up to 1 MiB. Each Gemma call can take up to `LILX_MODEL_TIMEOUT_SECONDS`; a full request allows two Gemma calls plus 30 seconds for backend work and overhead. Backend calls run concurrently through a shared pool of `LILX_WORKERS` goroutines; lilx waits for all selected calls before composing the result. OpenAPI `$ref` parameters and write operations are not supported yet. Backend credentials stay on the server. lilx sends selected backend JSON to the configured Gemma endpoint to compose the response; only connect data sources appropriate for that processing. The schema check verifies shape and types, not whether model-written statements are fully supported by the backend data. Review the trace before treating generated summaries as authoritative.

Each authorized request writes `LILX_LOG_DIR/<X-Lilx-Request-ID>.jsonl`. Entries include the client request and response, plus the full request and response bodies for each Gemma and backend call. Failed Gemma calls record the exact HTTP error response or transport error in the `model_response.error` field. Concurrent backend replies may appear in completion order. Authorization and API key headers are omitted; credential-like URL query parameters are redacted. Logs use mode `0600`, and the default `logs/` directory is gitignored. These files contain prompts and raw backend data, so keep the log directory private and delete logs when no longer needed. Responses above the existing 1 MiB backend or 128 KiB model limits are logged only up to those limits.
