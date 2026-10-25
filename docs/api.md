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

Check the configured AI adapter and model. The existing endpoint uses the adapter selected in `.env`. This endpoint requires the same bearer token as the others when `LILX_API_KEY` is set.

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
{"text":"Hello from the model."}
```

Invalid input returns `400` plain text. A model failure returns `502` with `{"error":"model request failed"}`. Authentication and logging errors return `401` and `500` as above.

## `POST /v1/ollama/test`

Test the local Ollama adapter using the same `{"prompt":"Reply with one short sentence."}` body and `{"text":"..."}` response as the model test above. Set `LILX_AI_PROVIDER="ollama"` and `LILX_MODEL="qwen3.5:4b"` in `.env`, start Ollama with that model installed, and restart lilx. The endpoint uses the same caller authentication, logging, input limits, and model timeout.

If another adapter is selected, it returns `409` with an explanation and makes no model call. Other statuses match `/v1/gemma/test`.

Use **Ollama test** in the Bruno collection, or:

```sh
curl --fail-with-body http://127.0.0.1:8080/v1/ollama/test \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"Reply with one short sentence."}'
```

## Server settings

| Variable | Purpose |
| --- | --- |
| `LILX_BACKEND_URL` | Required backend base HTTP(S) URL. |
| `LILX_OPENAPI_URL` | Required OpenAPI 3.x JSON or YAML URL. |
| `LILX_ALLOWED_OPERATIONS` | Required list of approved IDs, or `*`. |
| `GEMINI_API_KEY` | Server-side Google key; required only for `gemini`. Never sent to Ollama. |
| `LILX_API_KEY` | Optional caller bearer token. |
| `LILX_BACKEND_AUTHORIZATION` | Optional complete backend `Authorization` header. |
| `LILX_OPENAPI_AUTHORIZATION` | Optional complete `Authorization` header for the specification request. |
| `LILX_AI_PROVIDER` | `gemini` (default) or `ollama`. Unknown values fail startup. |
| `LILX_MODEL` | Gemini: `gemma-4-26b-a4b-it` (default) or `gemma-4-31b-it`. Ollama: installed model tag, default `qwen3.5:4b`. |
| `LILX_OLLAMA_URL` | Ollama base URL, default `http://127.0.0.1:11434`. HTTP(S), without credentials, query, or fragment. |
| `LILX_MODEL_TIMEOUT_SECONDS` | Model call timeout, default `600` (10 minutes), range `1`–`600`; also bounded by the full request deadline. |
| `LILX_WORKERS` | Shared backend call workers, default `10`, range `1`–`100`. |
| `LILX_LOG_DIR` | Request log directory, default `logs`. |
| `LILX_LISTEN_ADDR` | Listen address, default `127.0.0.1:8080`. |

Each authorized request writes a JSONL log named by its `X-Lilx-Request-ID` response header. Logs include client, backend, and model request and response bodies. Model exchanges include `provider` and `model`, and record exact HTTP, connection, and response decoding errors for either adapter. Validation failures produce `model_validation_error` entries with the stage, attempt, and exact validation error. Server error retries produce `model_retry` entries. Authorization headers and credential-like URL query parameters are omitted or redacted, but prompts and raw backend data remain in the logs. Keep the log directory private and delete records when no longer needed. Log files use mode `0600`; the default directory is ignored by Git.

## Choosing an AI adapter

Copy `.env.example` to `.env` for a new setup; preserve an existing `.env` and its credentials. `./start.sh` loads this file. Restart lilx after changing the provider or model.

For Gemini:

```dotenv
LILX_AI_PROVIDER="gemini"
LILX_MODEL="gemma-4-26b-a4b-it"
GEMINI_API_KEY="your-google-api-key"
```

For local Qwen, first start Ollama and install the model with `ollama pull qwen3.5:4b`, then set:

```dotenv
LILX_AI_PROVIDER="ollama"
LILX_MODEL="qwen3.5:4b"
LILX_OLLAMA_URL="http://127.0.0.1:11434"
```

Set both provider and model when switching an existing configuration. Gemini credentials can remain in `.env`; Ollama does not use them. Model selection is server configuration, not caller input. There is no automatic fallback between providers.

The Ollama adapter uses [`POST /api/chat`](https://docs.ollama.com/api/chat) with `stream: false`, `think: false`, temperature `0`, and a JSON Schema in `format` for planning and composition. It keeps the model loaded for 30 minutes after each call to avoid reload delays, using local memory during that period. The plain-text model test omits `format`. Use models that support these options; Qwen3.5 is the default. Gemini keeps the schema in its prompt. Both adapters use the same Go validation, permissions, retries, and timeouts.
