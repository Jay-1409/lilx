# lilx

lilx is an HTTP service that turns a natural-language data request into approved backend API calls. It reads an OpenAPI 3.x spec at startup, exposes the approved GET operations, asks Gemma 4 to choose calls, fetches their JSON responses, and returns data validated against the caller's JSON Schema.

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
| `LILX_ALLOWED_OPERATIONS` | Comma-separated OpenAPI `operationId` values approved for lilx |
| `LILX_API_KEY` | Bearer token required from lilx callers |
| `GEMINI_API_KEY` | Server-side key for Google Generative Language |
| `LILX_BACKEND_AUTHORIZATION` | Optional complete `Authorization` header sent to the backend |
| `LILX_OPENAPI_AUTHORIZATION` | Optional complete `Authorization` header used to fetch the spec |
| `LILX_MODEL` | Optional: `gemma-4-26b-a4b-it` (default) or `gemma-4-31b-it` |
| `LILX_LISTEN_ADDR` | Optional listen address (default `:8080`) |

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

Send `Authorization: Bearer <LILX_API_KEY>` to both product endpoints. A successful response contains `data` in the requested shape and a separate `trace` with selected operation IDs and HTTP statuses. Invalid plans, upstream failures, and output that does not match the schema return errors.

## Test Gemma with Bruno

Open the [`bruno`](bruno) folder as a Bruno collection and select the `local` environment. Set the secret `lilxApiKey` in Bruno to the same value as the server's `LILX_API_KEY`, then run **Gemma test**. The request calls `POST /v1/gemma/test` with a short prompt; the response is `{"text":"..."}`. The server uses its configured Gemma model and `GEMINI_API_KEY`, so the Google key never enters the Bruno collection. This test endpoint requires the same bearer token as the other lilx endpoints.

The current execution boundary supports inline path and query parameters on approved GET operations, at most four calls per request, JSON backend responses up to 1 MiB, and a 45-second request deadline. OpenAPI `$ref` parameters and write operations are not supported yet. Backend credentials stay on the server. lilx sends selected backend JSON to the configured Gemma endpoint to compose the response; only connect data sources appropriate for that processing. The schema check verifies shape and types, not whether model-written statements are fully supported by the backend data. Review the trace before treating generated summaries as authoritative.
