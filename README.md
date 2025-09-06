# LLM Proxy

LLM Proxy is a lightweight HTTP service that forwards user prompts to the
OpenAI **Responses API**. It exposes a single endpoint that requires a shared
secret and is intended to simplify integrating language model responses into
other services without embedding API credentials in each client.

## Features

- Minimal HTTP server that accepts `GET /?prompt=...&key=...` requests
- Choose the **OpenAI model** per request via `model=...` (default: `gpt-4.1`)
- Optional per-request **web search** via `web_search=1|true|yes`
- Optional logging at `debug` or `info` levels
- Forwards requests to the OpenAI API using your existing API key
- Supports plain text, JSON, XML, or CSV responses

## Configuration

The service is configured entirely through command-line flags or environment
variables:

| Flag / Env                            | Description                                         |
|---------------------------------------|-----------------------------------------------------|
| `--service_secret` / `SERVICE_SECRET` | Shared secret required in the `key` query parameter |
| `--openai_api_key` / `OPENAI_API_KEY` | OpenAI API key used for requests                    |
| `--port` / `HTTP_PORT`                | Port for the HTTP server (default `8080`)           |
| `--log_level` / `LOG_LEVEL`           | `debug` or `info` (default `info`)                  |
| `--system_prompt` / `SYSTEM_PROMPT`   | Optional system prompt text                         |
| `--workers` / `GPT_WORKERS`           | Number of worker goroutines (default `4`)           |
| `--queue_size` / `GPT_QUEUE_SIZE`     | Request queue size (default `100`)                  |

> **Note:** Web search is **per request**, enabled by adding `web_search=1` to your query.

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

### Basic request (default model, no web search)

```shell
curl --get \
  --data-urlencode "prompt=Hello, how are you?" \
  --data-urlencode "key=mysecret" \
  "http://localhost:8080/"
```

### Choose a model

```shell
curl --get \
  --data-urlencode "prompt=Summarize quantum error correction" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "model=gpt-4o" \
  "http://localhost:8080/"
```

### Enable web search

```shell
curl --get \
  --data-urlencode "prompt=What changed in the 2025 child tax credit?" \
  --data-urlencode "key=mysecret" \
  --data-urlencode "web_search=1" \
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
  &model=MODEL_NAME         # optional; defaults to gpt-4.1
  &web_search=1|true|yes    # optional; enables OpenAI web_search tool
  &format=CONTENT_TYPE      # optional; or use Accept header
```

Supported models include any listed in `/v1/models` from the OpenAI API
(e.g. `gpt-4o`, `gpt-4o-mini`, `gpt-4.1`).
Not all models support tools; for **web search**, use `gpt-4o` or `gpt-4.1`.

### Model capabilities

| Model         | Provider | Web Search |
|---------------|----------|------------|
| `gpt-4.1`     | OpenAI   | Yes        |
| `gpt-4o`      | OpenAI   | Yes        |
| `gpt-4o-mini` | OpenAI   | No         |
| `gpt-5`       | OpenAI   | Yes        |
| `gpt-5-mini`  | OpenAI   | No         |

### Status codes

* `200 OK` – success
* `400 Bad Request` – missing required parameters or unknown model
* `403 Forbidden` – missing or invalid `key`
* `504 Gateway Timeout` – upstream request timed out
* `502 Bad Gateway` – OpenAI API returned an error

## Security

* All requests must include the shared secret via `key=...`.
* Do not expose this service to the public internet without appropriate network controls.

## Releasing

To publish a new version:

1. Update `CHANGELOG.md` with a new section describing the release.
2. Commit the change.
3. Tag the commit and push both the branch and tag:

   ```bash
   git tag vX.Y.Z
   git push origin master
   git push origin vX.Y.Z
   ```

Tags that begin with `v` trigger the release workflow, which builds binaries and uses the matching changelog section as
release notes.

## License

This project is licensed under the MIT License. See [LICENSE](MIT-LICENSE) for
details.
