# serverless — Go AWS Lambda template for the notebooks

> Why Go on the Lambda OS-only runtime (`provided.al2023`)? See
> [docs/adr/0001-use-go-on-lambda-os-only-runtime.md](docs/adr/0001-use-go-on-lambda-os-only-runtime.md) (status: proposed).

A reusable, GET-only REST API that runs the logic of the notebooks in this
repository as a single AWS Lambda function written in Go. Because the Mistral
API key lives **only in the Lambda's environment**, web users can neither abuse
your LLM endpoint nor steal your key — they can only call the narrow, validated
GET endpoints below.

## layout

```
serverless/
├── cmd/
│   ├── server/main.go      # the Lambda: wires env → LLM client → routes
│   └── local/main.go       # same routes on localhost:8080 for easy testing
├── internal/
│   ├── lambdaapi/          # generic adapter: routes ↔ Lambda/http, JSON, errors
│   ├── mistral/            # LLM interface + minimal Mistral API client (net/http)
│   ├── dialogue/           # port of notebooks/01_dialogue.ipynb
│   └── hello/              # port of notebooks/00_hello_mistral.ipynb
├── template.yaml           # AWS SAM template (API Gateway + provided.al2023)
├── Makefile                # build / zip / test
└── README.md
```

## endpoints

| Endpoint | Notebook | Query params | Response |
|---|---|---|---|
| `GET /dialogue` | `01_dialogue.ipynb` | `message` (required, Sam's line), `transcript` (optional, base64url JSON from the previous response) | `{ "transcript": [ {turn, speaker, persona, content}, ... ] }` |
| `GET /hello` | `00_hello_mistral.ipynb` | `prompt` (required) | `{ "answer": "..." }` |

Only `GET` is accepted (405 otherwise); unknown paths give 404; bad input
gives 400 with a JSON `{"error": "..."}`.

### the dialogue endpoint

Each call advances the conversation by one round — **Sam (you) → Vera
(economist) → Anton (philosopher)** — and returns the full transcript. To
continue, echo the returned `transcript` back as the `transcript` query
parameter (base64url-encoded JSON). The function is stateless, so no session
store is needed.

```bash
# turn 1
curl -G 'https://<api-id>.execute-api.<region>.amazonaws.com/Prod/dialogue' \
  --data-urlencode 'message=What will AI do to jobs?'

# turn 2: pass the transcript from the previous response back
curl -G '...' --data-urlencode 'message=And what about meaning?' \
  --data-urlencode 'transcript=<base64url-of-previous-transcript>'
```

Safety rails (a web user can send anything, so inputs are capped):

- Sam's line: max 2000 chars, then truncated to **5 sentences** (as in the notebook)
- transcript: max `DIALOGUE_MAX_TURNS × 3` entries, must contain only complete
  rounds (a multiple of 3 entries); speakers must strictly alternate
  Sam → Vera → Anton, and turn numbers/personas are **recomputed server-side** —
  a caller cannot forge Vera's or Anton's lines
- rounds capped at `DIALOGUE_MAX_TURNS` (default 3 = `N_TURNS` in the notebook,
  hard ceiling 10)
- each model reply: `max_tokens=512`, capped at 5 sentences in code

## add a notebook

1. create `internal/<name>/<name.go>` with a `Handler(llm mistral.LLM) lambdaapi.HandlerFunc`
   (copy `internal/hello` — it is the minimal example);
2. register one line in `cmd/server/main.go` (and `cmd/local/main.go`):
   `router.Add("/<name>", <name>.Handler(llm))`;
3. add a `GET` event for `/<name>` in `template.yaml`;
4. write table-driven tests with a fake `mistral.LLM` — no API key needed.

## test

requires Go as pinned in `go.mod` (currently 1.27.4, same as CI). An older local
Go works too: with the default `GOTOOLCHAIN=auto` it downloads the pinned
toolchain once (needs network; set `GOTOOLCHAIN=local` to forbid that).

```bash
go test ./...        # unit tests, all with fake LLMs / httptest servers
go vet ./...
```

Run the real API locally (needs your key in the environment, never in git — in
this repo [direnv](https://direnv.net/) loads `MISTRAL_API_KEY` from the root
`.env` automatically, no manual export needed):

```bash
go run ./cmd/local   # binds 127.0.0.1:8080 only — loopback, not exposed
curl 'http://localhost:8080/hello?prompt=What%20is%20a%20PoC%3F'
```

## deploy

```bash
make zip             # builds linux/arm64 `bootstrap` + function.zip
sam deploy --guided  # uses template.yaml; set MISTRAL_API_KEY when prompted
```

The Lambda runs on the `provided.al2023` custom runtime, so the binary must be
named `bootstrap` (the Makefile does this). Store the key in AWS Secrets Manager
and reference it with a `{{resolve:secretsmanager:...}}` dynamic reference rather
than passing it as a plain template parameter in production.

## environment variables (Lambda)

| Variable | Default | Purpose |
|---|---|---|
| `MISTRAL_API_KEY` | (required) | read at runtime from the Lambda env var (`os.Getenv`); **never baked into the `bootstrap` binary** and never sent to clients |
| `MISTRAL_MODEL` | `mistral-small-latest` | same default as `mistral_jupyter` |
| `MISTRAL_BASE_URL` | `https://api.mistral.ai/v1` | override for tests/proxies |
| `DIALOGUE_MAX_TURNS` | `3` | dialogue rounds allowed (max 10) |
