# lilx product guidance

This repository is for **lilx**, a tool that turns a natural language data request into calls to available APIs and returns the collected data in a caller-requested JSON format. The current [product mock](.lavish/api-orchestrator-mock.html) illustrates the flow; its API names, endpoint, numbers, and responses are demo data, not implementation contracts.

## Product contract

A caller supplies two pieces of information in one request:

```json
{
  "nlp_data_required": "Which open customer tickets mention checkout failures, and what is their impact on orders?",
  "response_format": {
    "summary": "",
    "matching_tickets": 0,
    "order_error_rate_percent": 0,
    "active_incident": "",
    "sources": []
  }
}
```

- `nlp_data_required` states what data the caller wants.
- `response_format` states the JSON shape the caller expects back. The mock uses an example object as a template. Whether the production contract uses this template or formal JSON Schema is still undecided; do not silently treat one as the other.
- The response should contain the requested data in that shape. Do not add unrelated fields to the caller's data object. Operational metadata can be exposed separately.

## Expected user journey

1. Show the caller which APIs and operations are available to their workspace.
2. Accept and validate both request fields before execution.
3. Select relevant **approved** operations based on the request and available capabilities.
4. Call those APIs, collect their responses, and combine the data needed for the requested fields.
5. Return JSON matching the caller's requested shape. Make the chosen calls, data sources, and any gaps inspectable in an execution trace.

The product should make it clear when no available API can answer a field, an upstream call fails, or the requested shape cannot be satisfied. Never present guessed or demo values as retrieved data.

## Technical boundaries for implementation

- Keep an API catalog with operation descriptions, input/output shapes, and workspace access permissions. The caller must not be able to invoke an unregistered or unauthorized operation through prompt text.
- Treat prompts and upstream API responses as untrusted data. They may inform data selection but must not override access rules or execution limits.
- Keep API credentials on the server side. Do not include secrets or sensitive raw responses in the returned data or trace.
- Validate the incoming JSON, bound the number and duration of API calls, handle upstream errors, and validate the final JSON against the chosen `response_format` semantics.
- Record enough trace information to explain which operations ran, their status, and which sources support returned values, while redacting sensitive data.

## AI model calls

Use the Google Generative Language `generateContent` endpoint for every AI inference in lilx, including request interpretation, API selection, data extraction, and response composition. The allowed Gemma 4 model IDs are:

- `gemma-4-31b-it`
- `gemma-4-26b-a4b-it`

Do not introduce another AI model or provider unless this product decision changes. The example below uses `gemma-4-26b-a4b-it`; replace that model ID with `gemma-4-31b-it` when that model is selected.

```bash
curl "https://generativelanguage.googleapis.com/v1beta/models/gemma-4-26b-a4b-it:generateContent" \
  -H 'Content-Type: application/json' \
  -X POST \
  -d '{
    "contents": [{
      "parts": [{"text": "Roses are red..."}]
    }]
  }'
```

The example shows the endpoint and `contents[].parts[].text` request shape. Configure authentication on the server side; never hardcode credentials in source, prompts, logs, or the client. Model output is a proposal: application code must still enforce API permissions and execution limits and validate the returned JSON.

## Decisions still open

- Exact request endpoint and transport.
- Exact `response_format` specification, including required fields, nullable values, nested structures, and how partial results or errors are represented.
- How API connectors are registered and how callers choose which connected APIs may be used.

Keep future product work aligned with the two-part request contract and the discover → select → call → collect → return flow. Do not bake the mock's Support, Orders, or Status APIs into the core product.
