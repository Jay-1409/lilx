# Contributing to lilx

Thanks for contributing. Keep changes focused and preserve lilx's request flow: discover approved operations, select relevant calls, collect data, and return JSON in the caller's requested shape.

## Set up locally

Install Go 1.25.6 or newer and Node.js, then clone the repository and download dependencies:

```sh
git clone https://github.com/Jay-1409/lilx.git
cd lilx
go mod download
```

The [README](README.md) shows how to run lilx with the [sample backend](test-backend/README.md). Keep API keys in your shell or a local ignored `.env` file. The sample backend serves fixed test data, not live business data.

## Make a change

- Create a focused branch and explain the user-visible behavior in your pull request.
- Keep the core product independent of sample backend API names and data.
- Keep `nlp_data_required` and `response_format` together in each request. The current implementation accepts a JSON Schema for `response_format`; an example object is not a schema. If this contract changes, update [API documentation](docs/api.md) and tests together.
- Let the model propose calls and output, while application code enforces approved operations, call limits, and final JSON validation. Keep credentials server-side and the execution trace separate from the caller's data object.
- Add or update tests for behavior that could regress, and update documentation when setup, endpoints, limits, or settings change.

## Check your work

Run both test suites from the repository root:

```sh
go test ./...
node --test test-backend/server.test.js
```

Before opening a pull request, review the diff for credentials and sensitive backend responses. Include the test results and note any checks you could not run.
