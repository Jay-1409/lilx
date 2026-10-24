# API and configuration

lilx reads an OpenAPI 3.x JSON or YAML document at startup. It exposes only approved GET operations from that document. Set `LILX_ALLOWED_OPERATIONS` to `*` or a comma-separated list of operation IDs. Operations without an `operationId` use `GET /path` as their ID. The backend and specification URLs are operator settings; callers cannot change them in a request.

## Authentication

Set `LILX_API_KEY` to require `Authorization: Bearer <key>` on all three endpoints. An unset key disables caller authentication. The default listener binds to `127.0.0.1:8080`.

## `GET /v1/operations`

List the approved operations. The request has no body or query parameters.

```sh
curl --fail-with-body http://127.0.0.1:8080/v1/operations
```

Success, `200 OK`:

```json
{"operations":[{"operation_id":"getCheckoutMetrics","path":"/metrics/checkout","summary":"Get checkout order counts and error rate"}]}
```

The exact list comes from the configured OpenAPI document. Each operation can include a description and documented path or query parameters. An invalid bearer token returns `401 unauthorized`. A request log failure returns `500 request logging unavailable`.

## `POST /v1/requests`

Ask for data and supply the required output shape. Send one JSON object with both fields.

| Parameter | Type | Description |
| --- | --- | --- |
| `nlp_data_required` | Nonempty string | The data question. |
| `response_format` | JSON Schema object | Must have `type: object`. Defines the returned `data` object. External `$ref` values are rejected. |

The request body limit is 64 KiB, and the schema limit is 16 KiB. The format is a JSON Schema in the current implementation. It is not an example value template.

```sh
curl --fail-with-body http://127.0.0.1:8080/v1/requests \
  -H 'Content-Type: application/json' \
  -d '{"nlp_data_required":"What is the checkout order error rate?","response_format":{"type":"object","properties":{"order_error_rate_percent":{"type":"number"}},"required":["order_error_rate_percent"],"additionalProperties":false}}'
```

Success, `200 OK` (illustrative shape; the number is not a live result):

```json
{"data":{"order_error_rate_percent":2.5},"trace":[{"operation_id":"getCheckoutMetrics","status":200}]}
```

`data` is validated against the supplied schema. `trace` reports selected operation IDs and HTTP statuses. It does not identify which source supports each individual value. Review generated summaries against source data before relying on them.

| Status | Meaning |
| --- | --- |
| `400` | Invalid JSON, missing question, or invalid schema. The response is plain text. |
| `401` | Missing or incorrect bearer token when authentication is enabled. |
| `413` | Request body exceeds the limit. |
| `422` | No approved operation can answer, or composed data fails schema validation. |
| `502` | Planning, backend call, or response composition failed. |
| `500` | Request logging is unavailable. |

Most execution errors use a JSON object with `error` and, when calls were selected, `trace`:

```json
{"error":"no approved operation can answer this request","trace":[]}
```

The service makes at most ten backend calls per request. It accepts documented inline path and query parameters on approved GET operations. It does not support write operations or OpenAPI `$ref` parameters. A backend call has a 15-second HTTP timeout and a 1 MiB response limit. A model call has a configurable timeout and a 128 KiB response limit. Each planning or composition stage gets at most two model attempts. Invalid JSON or schema output gets one correction attempt; an HTTP 5xx model error gets one retry after one second. The full request has a 10-minute deadline, shared across all model attempts and backend work. The model is instructed not to guess missing values. Schema validation checks shape and types, but does not prove that every model-written statement is supported by backend data.

## `POST /v1/gemma/test`

Check the configured Gemma model. This endpoint requires the same bearer token as the others when `LILX_API_KEY` is set.

| Parameter | Type | Description |
| --- | --- | --- |
| `prompt` | Nonempty string | Text to send to the configured model. The JSON body must be under 4 KiB. |

```sh
curl --fail-with-body http://127.0.0.1:8080/v1/gemma/test \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"Reply with one short sentence."}'
```

Success, `200 OK`:

```json
{"text":"Hello from Gemma."}
```

Invalid input returns `400` plain text. A model failure returns `502` with `{"error":"model request failed"}`. Authentication and logging errors return `401` and `500` as above.

## Server settings

| Variable | Purpose |
| --- | --- |
| `LILX_BACKEND_URL` | Required backend base HTTP(S) URL. |
| `LILX_OPENAPI_URL` | Required OpenAPI 3.x JSON or YAML URL. |
| `LILX_ALLOWED_OPERATIONS` | Required list of approved IDs, or `*`. |
| `GEMINI_API_KEY` | Required server-side Google Generative Language key. |
| `LILX_API_KEY` | Optional caller bearer token. |
| `LILX_BACKEND_AUTHORIZATION` | Optional complete backend `Authorization` header. |
| `LILX_OPENAPI_AUTHORIZATION` | Optional complete `Authorization` header for the specification request. |
| `LILX_MODEL` | `gemma-4-26b-a4b-it` by default; `gemma-4-31b-it` is also allowed. |
| `LILX_MODEL_TIMEOUT_SECONDS` | Model call timeout, default `600` (10 minutes), range `1`–`600`; also bounded by the full request deadline. |
| `LILX_WORKERS` | Shared backend call workers, default `10`, range `1`–`100`. |
| `LILX_LOG_DIR` | Request log directory, default `logs`. |
| `LILX_LISTEN_ADDR` | Listen address, default `127.0.0.1:8080`. |

Each authorized request writes a JSONL log named by its `X-Lilx-Request-ID` response header. Logs include client, backend, and model request and response bodies. Validation failures produce `model_validation_error` entries with the stage, attempt, and exact validation error. Server error retries produce `model_retry` entries. Authorization headers and credential-like URL query parameters are omitted or redacted, but prompts and raw backend data remain in the logs. Keep the log directory private and delete records when no longer needed. Log files use mode `0600`; the default directory is ignored by Git.
