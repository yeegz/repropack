# Contributing

Thanks for helping. Small, focused pull requests are easiest to review.

## Setup

```bash
git clone <your fork> && cd repropack
go test -race ./...                       # core module: stdlib only
cd desktop && go test -race ./...         # desktop backend
cd frontend && npm ci && npm run check && npm test
```

Preview the desktop UI in a browser with a mock backend:

```bash
cd desktop/frontend && npm run dev
```

Run the real app with live reload:

```bash
cd desktop && wails dev
```

## Ground rules

- **Core stays dependency-free.** `format`, `redact`, `sdk`, `otlp`, `ingest`, `viewer` and the CLI import only the Go standard library ([ADR 3](docs/adr/0003-stdlib-core.md)).
- **Reports are untrusted.** Any code reading a report goes through `format.Reader`, respects `Limits`, and never renders content as markup or executes it.
- **No secret capture, no upload.** Features must be explicit and visible to the person using them.
- **Format changes need a spec change.** Update `spec/SPEC.md`, the JSON Schemas, `CHANGELOG.md` and the examples (`go run ./internal/genexamples`) together, following [VERSIONING.md](docs/VERSIONING.md).
- **Tests come with the change.** For parsing code, add a malicious case or extend a fuzzer. For redaction, add both a positive and a negative example.
- **UI copy** uses sentence case and plain verbs, names things by what people understand, and gives errors that say what happened and what to do. No dot indicators, no all-caps labels.
- **Accessibility.** Every control is reachable by keyboard and has a name. Check reduced motion, high contrast and both colour schemes.

## Commit and review

- Run `gofmt`, `go vet ./...`, the tests above, and `svelte-check` with no errors.
- Describe *why* in the PR. Link to the spec section if the format is affected.
- Be kind. We follow the [Contributor Covenant 2.1](https://www.contributor-covenant.org/version/2/1/code_of_conduct/).
