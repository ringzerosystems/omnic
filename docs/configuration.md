# Omnic Configuration Guide

`omnic` and `omnic-handset` can be configured through command-line flags, environment variables, or a local `.env` file.

On startup, both binaries automatically look for a `.env` file in the working directory (see [`.env.sample`](../.env.sample)).

---

## Precedence Order

Configuration values resolve in the following order (highest priority first):

1. **CLI Flags** (e.g. `--addr :8080`)
2. **Environment Variables** (e.g. `OMNIC_ADDR=:8080`)
3. **Legacy Environment Variables** (e.g. `TWEMU_ADDR` or `TWPHONE_*` for backward compatibility)
4. **`.env` File** (loaded into the process environment if keys are unset)
5. **Built-in Defaults**

---

## Server Flags & Environment Variables (`omnic`)

| Flag | Env Variable | Default | Description |
| :--- | :--- | :--- | :--- |
| `--addr` | `OMNIC_ADDR` | `:8080` | Listen address for HTTP API. |
| `--engines` | `OMNIC_ENGINES` | `twilio` | Enabled engines: `twilio`, `whatsapp`, `telegram`, or `all` (comma-separated). |
| `--twilio-auth-token` | `OMNIC_TWILIO_AUTH_TOKEN` | `authtoken` | Secret token for Twilio HMAC-SHA1 webhook signature calculation. |
| `--twilio-upstream-url` | `OMNIC_TWILIO_UPSTREAM_URL` | *empty* | Upstream webhook URL where inbound Twilio SMS messages are forwarded. |
| `--twilio-upstream-method` | `OMNIC_TWILIO_UPSTREAM_METHOD` | `POST` | HTTP method for Twilio upstream forwarding (`POST` or `GET`). |
| `--twilio-account` | `OMNIC_TWILIO_ACCOUNT` | *empty* | Twilio account credentials: `AC<hex>:TOKEN` (can be specified multiple times). |
| `--twilio-number` | `OMNIC_TWILIO_NUMBER` | *empty* | Pre-provisioned Twilio phone number: `+E164[@AC...]` (can be repeated). |
| `--whatsapp-phone-id` | `OMNIC_WHATSAPP_PHONE_ID` | `100000000000001` | Meta WhatsApp Phone Number ID. |
| `--whatsapp-waba-id` | `OMNIC_WHATSAPP_WABA_ID` | `200000000000001` | Meta WhatsApp Business Account ID. |
| `--whatsapp-token` | `OMNIC_WHATSAPP_TOKEN` | `wa_token_dev` | Bearer access token for WhatsApp Graph API calls. |
| `--whatsapp-verify-token` | `OMNIC_WHATSAPP_VERIFY_TOKEN` | `wa_verify_token` | Webhook verification challenge token (`hub.verify_token`). |
| `--whatsapp-secret` | `OMNIC_WHATSAPP_SECRET` | `wa_secret_dev` | App secret for HMAC-SHA256 (`X-Hub-Signature-256`) signatures. |
| `--whatsapp-upstream-url` | `OMNIC_WHATSAPP_UPSTREAM_URL` | *empty* | Upstream webhook URL where inbound WhatsApp messages are forwarded. |
| `--telegram-bot-token` | `OMNIC_TELEGRAM_BOT_TOKEN` | `123456789:mock_token_dev` | Mock Telegram Bot token in URL paths `/bot<token>/...`. |
| `--telegram-bot-username` | `OMNIC_TELEGRAM_BOT_USERNAME` | `omnic_bot` | Bot username returned by `/bot<token>/getMe`. |
| `--telegram-bot-name` | `OMNIC_TELEGRAM_BOT_NAME` | `OmnicBot` | Friendly bot display name returned by `/bot<token>/getMe`. |
| `--telegram-upstream-url` | `OMNIC_TELEGRAM_UPSTREAM_URL` | *empty* | Upstream webhook URL where inbound Telegram updates are forwarded. |
| `--telegram-secret-token` | `OMNIC_TELEGRAM_SECRET_TOKEN` | *empty* | Secret token expected in `X-Telegram-Bot-Api-Secret-Token` header. |
| `--telegram-delivery-delay` | `OMNIC_TELEGRAM_DELIVERY_DELAY` | `20ms` | Simulated delivery latency for Telegram webhook updates. |
| `--delivery-delay` | `OMNIC_DELIVERY_DELAY` | `50ms` | Simulated carrier / transit latency for SMS state transitions. |
| `--fail-rule` | `OMNIC_FAIL_RULE` | *empty* | Repeatable fault injection rule (see syntax below). |
| `--db` | `OMNIC_DB` | *empty* | Path to SQLite database file. If omitted, uses in-memory storage. |
| `--dev` | `OMNIC_DEV` | `false` | Enable human-readable colored console logging (Zap development logger). |
| `--pprof` | `OMNIC_PPROF` | `localhost:6060` | Internal listen address for pprof profiling and Prometheus `/metrics`. |
| `--seed` | `OMNIC_SEED` | `false` | Pre-seed demo phone numbers on startup. |
| `--version` | `OMNIC_VERSION` | `false` | Print binary version and exit. |

---

## Handset Flags & Environment Variables (`omnic-handset`)

| Flag | Env Variable | Default | Description |
| :--- | :--- | :--- | :--- |
| `--engine` | `OMPHONE_ENGINE` | `all` | Handset protocol engine mode: `twilio`, `whatsapp`, `telegram`, or `all`. |
| `--server` | `OMPHONE_SERVER` | `http://localhost:8080` | `omnic` server base URL. |
| `--from` | `OMPHONE_FROM` | `+14155551111` | Sender phone number representing the handset. |
| `--to` | `OMPHONE_TO` | `+14155550199` | Recipient phone number representing the agent / application. |
| `--telegram-chat-id` | `OMPHONE_TELEGRAM_CHAT_ID` | `14155551111` | Telegram chat ID representing the handset user. |
| `--telegram-username` | `OMPHONE_TELEGRAM_USERNAME` | `handset_user` | Telegram username of handset participant. |
| `--audio-in` | `OMPHONE_AUDIO_IN` | `default` | Audio capture device ID or name. |
| `--audio-out` | `OMPHONE_AUDIO_OUT` | `default` | Audio playback device ID or name. |
| `--list-audio-devices` | N/A | `false` | Discover host audio input/output devices and exit. |
| `--body` | N/A | *empty* | Message body for scriptable one-shot CLI send. |
| `--headless` | N/A | `false` | Run in headless streaming mode (no TUI, prints to stdout). |
| `--version` | N/A | `false` | Print handset version and exit. |

---

## Fault Injection & Carrier Simulation Rules

`omnic` includes a fault injection engine to simulate carrier network outages, rate limits, spam filtering, and webhook timeouts.

### Rule Syntax

```text
<operation>=<action>[@<qualifier>]
```

#### Operations

- `send`: Applies to outbound message dispatch (`client.messages.create` or Graph API `messages`).
- `webhook`: Applies to upstream webhook forwarding.

#### Actions

- `<http_code>:<twilio_error_code>`: Returns HTTP error and Twilio error code payload.
- `timeout`: Deliberately holds webhook requests past the standard 15-second client timeout window.
- `delay:<duration>`: Injects custom artificial delay (e.g. `delay:5s`).

#### Qualifiers

- `@to <phone_number>`: Applies only when sending to a specific destination number.
- `@after <count>`: Triggers only after `<count>` successful messages have been processed.

### Example Rules

```bash
# Return 429 Too Many Requests (Twilio error 20429) after 5 successful messages
--fail-rule "send=429:20429@after 5"

# Reject messages to +15550009999 as spam/filtered (Twilio error 30007)
--fail-rule "send=400:30007@to +15550009999"

# Simulate an upstream agent webhook timeout
--fail-rule "webhook=timeout"
```

### Dynamic Simulation Rules API

Simulation rules can also be modified dynamically during automated test execution via REST without restarting the server:

```bash
# Register failure rules
curl -X POST http://localhost:8080/emu/simulate/rules \
  -H "Content-Type: application/json" \
  -d '{
    "rules": [
      {
        "operation": "send",
        "status_code": 400,
        "error_code": 30007,
        "match_to": "+15550009999"
      }
    ]
  }'

# Inspect active rules
curl http://localhost:8080/emu/simulate/rules

# Clear all active rules
curl -X DELETE http://localhost:8080/emu/simulate/rules
```
