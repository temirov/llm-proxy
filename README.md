# LLM Proxy

LLM Proxy is a lightweight HTTP service that forwards user prompts to the
OpenAI Chat Completion API. It exposes a single endpoint that requires a shared
secret and is intended to simplify integrating language model responses into
other services without embedding API credentials in each client.

## Features

- Minimal HTTP server that accepts `GET /?prompt=...&key=...` requests
- Optional logging at `debug` or `info` levels
- Forwards requests to the OpenAI API using your existing API key

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

## Running

A secret can be easily generated with the following command

```shell
openssl rand -hex 32
```

The service running command is

```shell
SERVICE_SECRET=mysecret OPENAI_API_KEY=sk-xxxxx \
  ./llm-proxy --port=8080 --log_level=info
```

Once running, send a request with the secret key:

```shell
curl --get \
  --data-urlencode "prompt=Hello, how are you?" \
  --data-urlencode "key=mysecret" \
  "http://localhost:8080/"
```

The response body contains the model's reply as plain text.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for
details.
