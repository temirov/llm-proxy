# LLM Proxy

LLM Proxy is a lightweight HTTP service that forwards user prompts to the
OpenAI **Responses API**. It exposes a single endpoint that requires a shared
secret and is intended to simplify integrating language model responses into
other services without embedding API credentials in each client.

## Features

- Minimal HTTP server that accepts `GET /?prompt=...&key=...` requests
- **Optional web search per request** via `web_search=1|true|yes`
- Optional logging at `debug` or `info` levels
- Forwards requests to the OpenAI API using your existing API key
- Returns plain text by default; supports JSON, XML, or CSV via `format` or `Accept`

## How it works

- By default, the proxy calls the OpenAI **Responses API** with your prompt and (optional) system prompt.
- If the request includes `web_search=1` (or `true` / `yes`), the proxy adds the built-in `{"type":"web_search"}` tool to the API payload, enabling browsing and source-backed answers.

## Configuration

The service is configured entirely through command-line flags or environment
variables:

| Flag / Env                            | Description                                         |
|---------------------------------------|-----------------------------------------------------|
| `--service_secret` / `SERVICE_SECRET` | Shared secret required in the `key` query parameter |
| `--openai_api_key` / `OPENAI_API_KEY` | OpenAI API key used for requests                    |
| `--port` / `GPT_PORT`                 | Port for the HTTP server (default `8080`)           |
| `--log_level` / `LOG_LEVEL`           | `debug` or `info` (default `info`)                  |
| `--system_prompt` / `SYSTEM_PROMPT`   | Optional system prompt text                         |
| `--workers` / `GPT_WORKERS`           | Number of worker goroutines (default `4`)           |
| `--queue_size` / `GPT_QUEUE_SIZE`     | Request queue size (default `100`)                  |

> **Note:** Web search is **per request**, controlled by the `web_search` query parameter. No extra server flags are required.

## Running

Generate a secret:

```shell
openssl rand -hex 32
````

Run the service:

```shell
SERVICE_SECRET=mysecret OPENAI_API_KEY=sk-xxxxx \
  ./llm-proxy --port=8080 --log_level=info
```

## Usage

### Basic request (no web search)

```shell
curl --get \
  --data-urlencode "prompt=Hello, how are you?" \
  --data-urlencode "key=mysecret" \
  "http://localhost:8080/"
```

### Enable web search for this request

```shell
curl --get \
  --data-urlencode "prompt=What changed in the 2025 child tax credit?" \
  --data-urlencode "web_search=1" \
  --data-urlencode "key=mysecret" \
  "http://localhost:8080/"
```

### Response formats

You can request alternative formats using either the `format` query parameter or
the `Accept` header. Supported values are:

* `text/csv` – the reply as a single CSV cell with internal quotes doubled
  and a trailing newline
* `application/json` – JSON object containing `request` and `response` fields
* `application/xml` – XML document `<response request="...">...</response>`

If no supported value is provided, `text/plain` is returned.

## Endpoint

```
GET /
  ?prompt=STRING            # required
  &key=SERVICE_SECRET       # required
  &web_search=1|true|yes    # optional; enables OpenAI web_search tool
  &format=CONTENT_TYPE      # optional; or use Accept header
```

### Status codes

* `200 OK` – success
* `400 Bad Request` – missing required parameters (e.g., `prompt`)
* `403 Forbidden` – missing or invalid `key`
* `504 Gateway Timeout` – upstream request timed out
* `502 Bad Gateway` – OpenAI API returned an error

## Security

* All requests must include the shared secret via `key=...`.
* Do not expose this service to the public internet without appropriate network controls (IP allowlist, private ingress, auth proxy, etc.).

## License

This project is licensed under the MIT License. See [LICENSE](MIT-LICENSE) for
details.
