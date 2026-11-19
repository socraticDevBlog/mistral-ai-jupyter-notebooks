"""Shared Mistral AI client setup for the proof-of-concept notebooks."""

import os
from collections.abc import Iterator
from pathlib import Path
from typing import Any

from dotenv import load_dotenv

try:  # mistralai < 3
    from mistralai import Mistral
except ImportError:  # mistralai >= 3
    from mistralai.client import Mistral

DEFAULT_MODEL = "mistral-small-latest"

# Per-request ceiling for PoC runs, so a runaway notebook cannot burn huge
# completions. Raise it here deliberately if an experiment needs more.
MAX_TOKENS = 4096

PROJECT_ROOT = Path(__file__).resolve().parent.parent


def get_client() -> Mistral:
    """Build a Mistral client from MISTRAL_API_KEY.

    Reads the project root .env explicitly (never a parent directory's);
    a real environment variable always takes precedence over the file.
    """
    load_dotenv(PROJECT_ROOT / ".env")
    api_key = os.environ.get("MISTRAL_API_KEY")
    if not api_key:
        raise RuntimeError(
            "MISTRAL_API_KEY is not set. Copy .env.example to .env and add your key."
        )
    return Mistral(api_key=api_key)


def _check_budget(max_tokens: int) -> None:
    if max_tokens > MAX_TOKENS:
        raise ValueError(
            f"max_tokens={max_tokens} exceeds the project cap MAX_TOKENS={MAX_TOKENS}. "
            "Raise MAX_TOKENS in mistral_jupyter/mistral_client.py if needed."
        )


def complete(
    messages: list[dict[str, str]],
    *,
    model: str = DEFAULT_MODEL,
    max_tokens: int = MAX_TOKENS,
    client: Mistral | None = None,
    **kwargs: Any,
) -> Any:
    """One-shot chat completion capped at MAX_TOKENS.

    Returns the raw response; the text is at `.choices[0].message.content`.
    """
    _check_budget(max_tokens)
    client = client or get_client()
    return client.chat.complete(
        model=model, messages=messages, max_tokens=max_tokens, **kwargs
    )


def stream(
    messages: list[dict[str, str]],
    *,
    model: str = DEFAULT_MODEL,
    max_tokens: int = MAX_TOKENS,
    client: Mistral | None = None,
    **kwargs: Any,
) -> Iterator[str]:
    """Streaming chat completion capped at MAX_TOKENS; yields text deltas."""
    _check_budget(max_tokens)
    client = client or get_client()
    chunks = client.chat.stream(
        model=model, messages=messages, max_tokens=max_tokens, **kwargs
    )
    for chunk in chunks:
        delta = chunk.data.choices[0].delta.content
        if delta:
            yield delta
