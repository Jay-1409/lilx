# Contributing

## Set up

Install Go 1.25.6 or newer and Node.js. Clone the repository and download Go dependencies.

```sh
git clone https://github.com/Jay-1409/lilx.git
cd lilx
go mod download
```

Read the root [README](../README.md) for the local service setup. Use the [sample backend](../test-backend/README.md) for manual requests. Keep API keys in your shell or a local ignored environment file.

## Branches and changes

Create a focused branch for each change. Use the `codex/` prefix for Codex-created branches. Keep the core product independent of the sample backend's API names and data.

When changing the request contract, update [API documentation](./api.md) and tests together. Keep `nlp_data_required` and `response_format` in the same request. Keep operational trace fields separate from the caller's data object.

## Test

Run the Go tests and the sample backend tests from the repository root.

```sh
go test ./...
node --test test-backend/server.test.js
```

## Pull request checklist

- Explain the user-visible change and any changed request or response shape.
- Add or update tests for behavior that could regress.
- Run both test commands above.
- Update documentation when setup, endpoints, limits, or settings change.
- Check that no credentials or sensitive backend responses entered the commit.
