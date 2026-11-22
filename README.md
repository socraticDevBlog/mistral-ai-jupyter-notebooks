![MistralAI](https://img.shields.io/badge/mistralai-%23FA520F.svg?style=for-the-badge&logo=mistralai&logoColor=white)![Jupyter Notebook](https://img.shields.io/badge/jupyter-%23FA0F00.svg?style=for-the-badge&logo=jupyter&logoColor=white)

![Light pencil drawing of an alchemist with his instruments and a sleeping cat, next to a windmill](assets/alchemist_windmill.png)

# Mistral.ai Jupyter Notebooks

Research playground for building proof-of-concepts with Jupyter notebooks on top of the [Mistral AI API](https://docs.mistral.ai/).

## layout

```
mistral-jupyter/
├── notebooks/          # proof-of-concept notebooks, numbered: 00_, 01_, 02_, ...
├── mistral_jupyter/    # installable python package shared by the notebooks
├── data/               # local datasets and generated artifacts (gitignored)
├── pyproject.toml      # dependencies (PEP 621) + build/lint config
├── uv.lock             # locked, hash-pinned dependency set (commit this)
├── .python-version     # pinned python (3.12)
├── .env.example        # copy to .env and set your MISTRAL_API_KEY
└── README.md
```

## setup

requirements: [uv](https://docs.astral.sh/uv/)

```bash
uv sync                 # creates .venv, installs uv.lock, and installs mistral_jupyter (editable)
cp .env.example .env    # then edit .env and paste your MISTRAL_API_KEY
uv run jupyter lab
```

then open `notebooks/00_hello_mistral.ipynb` and run all cells as a smoke test.

## direnv (.envrc)

this repo ships a `.envrc` that uses direnv's `dotenv` command to load `.env` (your `MISTRAL_API_KEY`) into the environment automatically when you `cd` into the project, and to unload it when you leave. after installing [direnv](https://direnv.net/) and adding `eval "$(direnv hook bash)"` (or `zsh`/`fish`/...) to your shell rc, run `direnv allow` once in the repo root — direnv refuses to execute an `.envrc` until you explicitly allow it, and you must re-run `direnv allow` whenever the file changes. only `.envrc` is committed (it holds no secrets, just the `dotenv` line plus a `# shellcheck shell=bash` directive, since direnv evaluates it with bash); the real key stays in the gitignored `.env`. use `direnv deny` to revoke access, or delete `.envrc` to opt out entirely. because direnv exports the variables into your shell, `uv run` and `get_client()` pick them up with no extra code.

## commands

```bash
uv run python ...                                             # run anything in the project env
uv add <package>                                              # add a dependency (updates pyproject.toml + uv.lock)
uv add --group dev <package>                                  # add a dev-only tool
uv run ruff check                                             # lint
uv run jupyter nbconvert --to notebook --execute notebooks/00_hello_mistral.ipynb   # headless notebook run
uv run nbstripout --install                                   # git filter: strips notebook outputs on commit
```

## pre-commit hooks (gitleaks, ruff, nbstripout)

hooks live in `.pre-commit-config.yaml` and run through [uvx](https://docs.astral.sh/uv/) — no global install needed. this repo is a monorepo, so install from the **repo root** with an explicit config path (the hooks are scoped to `mistral-jupyter/` only):

```bash
uvx pre-commit install -c mistral-jupyter/.pre-commit-config.yaml    # install the git hook
uvx pre-commit run --config mistral-jupyter/.pre-commit-config.yaml --all-files   # manual run
```

if you clone `mistral-jupyter/` as a standalone repo instead, run plain `uvx pre-commit install` and drop the `mistral-jupyter/` prefix from `files:` and the gitleaks `--config` path.

what the hooks do:

- **gitleaks** — blocks secrets from being committed; a custom `mistral-api-key` rule lives in `.gitleaks.toml` (the real key stays in the gitignored `.env`)
- **ruff + ruff-format** — lint/format python, including notebook cells
- **nbstripout** — strips notebook outputs on commit, so outputs never leak data into git
- hygiene hooks — trailing whitespace, end-of-file, yaml/toml validity, large files

optional — register the project venv as a named Jupyter kernel:

```bash
uv run python -m ipykernel install --user --name mistral-jupyter --display-name "Python 3 (mistral-jupyter)"
```

## conventions

- one notebook per proof-of-concept, prefixed with a number: `01_summarization.ipynb`, `02_rag.ipynb`, ...
- shared code (client setup, prompts, helpers) lives in the `mistral_jupyter` package; notebooks just `from mistral_jupyter import ...` — no `sys.path` hacks, works in Jupyter, scripts, and nbconvert alike
- secrets only in `.env` (gitignored); datasets and outputs go in `data/`
- keep notebooks runnable top-to-bottom; keep experiment knobs (model, temperature, ...) in the first code cell
- strip outputs before committing notebooks (`uv run nbstripout --install` does this automatically) — outputs can leak data

## security

- `MISTRAL_API_KEY` lives only in `.env` or your shell environment — never in notebooks, code, or git. `.gitignore` covers `.env` plus `.env.*` variants, `*.pem`/`*.key`, and ssh private keys.
- `get_client()` loads `.env` from the project root only (a parent directory's `.env` is never picked up), and a real environment variable always wins over the file.
- `uv.lock` pins exact versions and hashes for all 100+ packages, so `uv sync` installs a reproducible, tamper-evident dependency set.
- gitleaks runs on every commit (see pre-commit hooks below) and fails the commit if a secret — e.g. `MISTRAL_API_KEY` — is staged.
- if a key ever leaks (committed notebook output, screenshot, logs): rotate it immediately at <https://console.mistral.ai/api-keys/>.
- `uv run jupyter lab` binds to localhost with token auth by default — keep it that way. Never start the server with `--ip 0.0.0.0` and an empty token on a shared machine.
- don't `print(client)` or log raw responses when experimenting with sensitive prompts; the SDK's repr is safe, but response payloads are not.

## api key management

official website: <https://mistral.ai> — API docs at <https://docs.mistral.ai>, and keys are created/managed in the console (La Plateforme / Studio) at <https://console.mistral.ai/api-keys/>.

- create keys only in the console, never by hand. the full key is shown **once** at creation — copy it straight into `.env` (or a password manager); you cannot retrieve it later.
- keys are scoped to a workspace: use one workspace + key per environment (dev/staging/prod) so quota, rate limits, and revocation stay isolated.
- set an expiration date on throwaway/trial keys. keys do **not** expire on their own — the free tier is rate-limited but indefinite — and the expiration date cannot be changed after creation, so to extend a key, create a new one and delete the old. a non-expiring key is fine unless your org/workspace admin enforces an API key expiration policy (off by default; see <https://docs.mistral.ai/admin/identity-access/api-keys>).
- the default connector access scope (`shared connectors only`) is right for automation; only widen it if the app needs your private connectors.
- rotate on any suspected leak and delete unused keys from the console key list, which shows last-used and expiration per key. if a key ever leaks into git/notebooks, revoke it immediately at <https://console.mistral.ai/api-keys/> and issue a replacement.
- keep keys out of code and notebooks entirely: `.env` only (gitignored), gitleaks blocks secret commits, and a real environment variable always wins over the file.

## notes

- default model is `mistral-small-latest` — see `mistral_jupyter/mistral_client.py`; list everything available via the `models.list()` endpoint
- `MAX_TOKENS = 4096` caps every `complete()`/`stream()` call: asking for more raises a `ValueError` until you deliberately raise the cap in `mistral_jupyter/mistral_client.py` — a guard against runaway notebook spend
- API keys: <https://console.mistral.ai/api-keys/>
- SDK reference: <https://docs.mistral.ai/client/python/>

## License 


This work is licensed under a
[Creative Commons Attribution 4.0 International License][cc-by].

[![CC BY 4.0][cc-by-image]][cc-by]

[cc-by]: http://creativecommons.org/licenses/by/4.0/
[cc-by-image]: https://i.creativecommons.org/l/by/4.0/88x31.png
