# Errors and limits

Errors use the OpenAI format: an HTTP status code plus a JSON body with `error.message` and `error.code`. The message tells you what went wrong; the status code tells you what to do.

## Status codes

| Code | Meaning | What to do |
|---|---|---|
| 400 | The request is invalid, for example a malformed body or a parameter the model does not accept | Fix the request; retrying the same request will fail again |
| 401 | The API key is missing, wrong, disabled, expired, or has used up its own quota | Check the `Authorization: Bearer sk-...` header and the key's status on the [API Keys](/keys) page |
| 402 | Payment required. ADTY AI reports insufficient balance as 403 (below), so a 402 normally comes from the upstream provider | Retry later; contact support if it persists |
| 403 | Your balance is insufficient (`insufficient_user_quota`), or the key is not allowed to use this model or group | [Top up](/wallet), or check the key's model and group settings |
| 404 | Wrong path, or the model does not exist | Check the address (`/v1` included or not, see the client guides) and the model name on the [Pricing](/pricing) page |
| 429 | Too many requests for your account (`rate_limit_exceeded`), or the upstream is busy | Wait and retry; see the limits below |
| 500, 502, 503 | The upstream provider failed or no channel is available for the model | Retry with backoff; contact support if it persists |
| 524 | The request took too long and the connection was closed (see below) | Use streaming |

Requests rejected with an error are not charged.

## Per-account limits

An account can have three limits, counted across all of its keys:

- **RPM**: requests per minute.
- **TPM**: tokens per minute.
- **Concurrency**: requests running at the same time.

Exceeding any of them returns 429 with a `Retry-After` header that says how many seconds to wait. Retry after that time instead of immediately. If your workload needs higher limits, contact support.

## Use streaming for long requests

Traffic passes through Cloudflare, which closes a connection that sends nothing for about **125 seconds**. A non-streaming request that takes longer than that (long outputs, reasoning models, big code edits) is cut off and your client sees an error, usually 524, even though the model was still working.

Set `"stream": true`. A streaming response starts sending data right away and keeps the connection alive until the model finishes. Claude Code, Codex CLI and most chat clients already stream by default.
