# ADR 0001 — Use Go on the Lambda "OS-only" runtime for the serverless API

- **Status:** proposed
- **Date:** 2026-11-20

## Context

The notebooks in this repo call the Mistral API with a secret key and a
carefully crafted harness (personas, labeled-transcript formatting, sentence
caps). If that logic lived in the browser, both the `MISTRAL_API_KEY` and the
prompt engineering would be public. We therefore want a thin, GET-only REST
API backed by AWS Lambda, so the key and the harness never leave the server.

For compiled languages, AWS Lambda offers **OS-only runtimes**
(`provided.al2023`): you upload a static `bootstrap` binary and AWS runs it on
a bare Amazon Linux image — no language runtime to manage. The realistic
candidates are Go and Rust. The team is fluent in Python (the notebooks), but
for this server-side component we prefer a compiled language over Python.

The `MISTRAL_API_KEY` is **never baked into the binary**: `make zip` builds a
static `bootstrap` with no secrets in it, and the running function reads the
key at cold start from the Lambda environment variable (`os.Getenv`, see
`internal/mistral/mistral.go`). In `template.yaml` the variable is injected
from a `NoEcho` CloudFormation parameter — in production, prefer a Secrets
Manager dynamic reference (`{{resolve:secretsmanager:...}}`).

## Decision

Write the Lambda in **Go**, cross-compiled to a static `linux/arm64` binary
(`make zip`) and deployed on `provided.al2023`.

**Why Go over Rust:** Rust executes faster, but this workload is I/O-bound —
each invocation spends its time waiting on the Mistral HTTP API, so Rust's
speed advantage buys nothing here. Go wins on the axes that actually matter
for us: fast builds, simple concurrency, a mature Lambda ecosystem
(`aws-lambda-go`), and a gentler learning curve. The decisive requirement —
keeping the API key and prompt harness server-side — is satisfied equally by
any compiled language, so Rust's performance is not worth its steeper cost.

**Why a compiled language over Python:** a single static binary needs no
dependency bundling into the deployment zip, starts fast, and is at least
mildly harder to read than shipped Python source. (Honesty: compilation is
*not* a security boundary — the real protection is that the binary never
leaves the server; treat obfuscation as a bonus, not a control.)

## Drawbacks of Go (listed candidly)

- **RE2 regexps have no lookbehind**, so the notebook's sentence-splitting
  regex had to be re-implemented by hand (`SplitSentences`).
- **Verbose error handling** — every call returns an explicit `error`; the
  code is noisier than Python's exceptions.
- **Thin stdlib for our needs** — no built-in HTTP retry/backoff or chat
  client; the Mistral client is hand-rolled `net/http`.
- **~8 MB binary** — fine against Lambda's limits, but heavier than it needs
  to be.
- **Team ramp-up** — we know Python, not Go; expect slower reviews until the
  team is idiomatic.
- **Cold-start/init discipline** — package-level init (env parsing, client
  construction) runs per cold start; keep it cheap (it is: one struct).

## Consequences

- **Positive:** one static `bootstrap` binary on an OS-only runtime; millisecond
  cold starts; small zip; the API key and prompt harness stay server-side;
  handlers depend on a narrow `mistral.LLM` interface, so unit tests run with
  fakes and no credentials (`go test ./...`, gated by CI).
- **Negative:** the team maintains Go code alongside Python; the drawbacks
  above apply. If a future notebook ever becomes CPU-bound (e.g. heavy local
  text processing), revisit Rust — this ADR covers the LLM-proxy use case only.

## Alternatives considered

- **Rust on `provided.al2023`** — rejected: faster execution is irrelevant for
  an I/O-bound proxy; slower builds and steeper learning curve.
- **Python on the `python3.x` runtime** — rejected for the server: interpreted,
  requires bundling dependencies into the zip, and ships readable source.
  Python remains the right choice for the notebooks themselves.
