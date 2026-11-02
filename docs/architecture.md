# Omnic Architecture & System Design

`omnic` is designed as a concurrent, high-performance omnichannel communications emulator. It simulates Twilio Programmable Messaging, Meta WhatsApp Cloud API, and Telegram Bot API with wire-level parity for local development, automated testing, and conversational AI agent evaluation.

---

## Hexagonal Layering

`omnic` enforces strict hexagonal architecture (ports and adapters), validated hermetically by `arch-go`:

```text
cmd/omnic       ──────►  internal/handler  ──────►  internal/service  ──────►  internal/repository
cmd/omnic-handset                │                         │                         │
                                 └───────────► internal/domain ◄─────────────────────┘
```

### Layer Responsibilities

| Package | Role | Dependencies & Boundaries |
| :--- | :--- | :--- |
| **`internal/domain`** | Core business domain | Standard library only (`entities.go`, `segment.go`). Zero external imports. Defines entities (`Message`, `PhoneNumber`, `MessagingService`), statuses, and segmentation logic. |
| **`internal/repository`** | Storage abstraction & adapters | Storage contracts (`interfaces.go`) with thread-safe implementations: memory (`internal/repository/memory`) and SQLite (`internal/repository/sqlite`). Never depends on transport or services. |
| **`internal/service`** | Core business logic | State machine dispatcher, message lifecycle, character encoding & segmentation, HMAC-SHA1/HMAC-SHA256 signature verification, TwiML parser, fault injection simulator, and event broker. |
| **`internal/handler`** | Transport / API Layer | Echo v4 HTTP routes, request binding, protocol serialization, SSE streaming, and health probes. Handlers never touch repository storage directly. |
| **`cmd/`** | Application Wiring | Entrypoints (`cmd/omnic`, `cmd/omnic-handset`). CLI flag parsing, configuration loading, dependency injection, and graceful shutdown orchestration. |

### Architectural Guardrails (`arch-go.yml`)

- `cmd/**` is wiring only; no cross-imports between separate binaries.
- `internal/**` never imports `cmd/**`.
- `internal/handler/**` never accesses `internal/storage/**` or repository internals directly.
- `internal/service/**` does not depend on `internal/handler/**`, Echo, or `database/sql`.
- Complexity caps: maximum 5 parameters, 3 return values, 80 lines per function, and strict public function limits per file.

---

## Conversational AI Agent Bridge

`omnic` enables full-duplex local development of AI agents, bots, and conversational workflows without external ingress tunnels (ngrok) or live carrier credentials.

```text
┌────────────────────────┐         Inbound SMS         ┌────────────────────────┐
│ omnic-handset (Handset)│ ──────────────────────────> │         omnic          │
│      +15551234567      │                             │         :8080          │
└────────────────────────┘                             └────────────────────────┘
            ▲                                                       │
            │                                                       │ Signed Webhook
            │ SSE Stream                                            │ POST /api/sms/inbound
            │ GET /emu/events                                       ▼
            │                                          ┌────────────────────────┐
            └───────────────────────────────────────── │    AI Agent Server     │
                     Outbound REST API Reply           │         :8000          │
                  POST /.../Messages.json              └────────────────────────┘
```

### End-to-End Execution Flow

1. **Inbound Forwarding**:
   - The handset client (or simulated sender via `/emu/inbound/...`) submits an inbound message.
   - `omnic` signs the HTTP POST payload with an authentic signature (`X-Twilio-Signature` using HMAC-SHA1 for Twilio, `X-Hub-Signature-256` using HMAC-SHA256 for WhatsApp, or `X-Telegram-Bot-Api-Secret-Token` for Telegram).
   - The signed payload is forwarded to the registered upstream webhook URL (`--twilio-upstream-url`, `--whatsapp-upstream-url`, or `--telegram-upstream-url`).

2. **Upstream Processing**:
   - The external AI agent server receives the webhook, verifies the cryptographic signature with standard SDKs, executes prompt chains / tool calls, and replies.
   - For Twilio, the agent can reply **synchronously** via TwiML XML (`<Response><Message>...</Message></Response>`) or return `200 OK` and reply **asynchronously** via Twilio REST API (`client.messages.create`).

3. **Asynchronous Outbound Reply**:
   - When the agent calls the REST API, `omnic` validates the request, runs GSM-7/UCS-2 segmentation, assigns a SID, and transitions message state: `queued` &rarr; `sending` &rarr; `sent` &rarr; `delivered`.
   - `omnic` triggers the agent's `StatusCallback` webhook to simulate carrier receipts.

4. **Real-Time Client Streaming**:
   - Outbound and inbound messages are published to `omnic`'s event broker.
   - Connected `omnic-handset` clients listening to `GET /emu/events` receive the message via Server-Sent Events (SSE) sub-millisecond, updating chat history in real time.

---

## Tri-Engine Protocol Design

`omnic` implements full protocol parity across three major messaging ecosystems:

```text
               ┌────────────────────────────────────────────────────────┐
               │                         omnic                          │
               └────────────────────────────────────────────────────────┘
                       │                        │                       │
         ┌─────────────┴────────────┐           │        ┌──────────────┴─────────────┐
         ▼                          ▼           ▼        ▼                            ▼
  Twilio REST API         Twilio Messaging   Meta WhatsApp Cloud API           Telegram Bot API
    v2010 Engine              v1 Engine          v21.0 / v26.0                      Engine
(/2010-04-01/Accounts)     (/v1/Services)   (/:version/:phone_id)            (/bot<token>/...)
```

1. **Twilio Engine**:
   - Emulates `/2010-04-01/Accounts/{AccountSid}/Messages.json` and Messaging Services (`/v1/Services`).
   - Supports form-urlencoded payloads, Basic Auth verification, SID formats (`SM...`, `MM...`, `MG...`, `PN...`), media attachments, and scheduled messages.
   - Exact GSM-7 character table and UCS-2 detection for SMS segmentation math.

2. **Meta WhatsApp Cloud API Engine**:
   - Dynamic API version routing supporting `v21.0` through `v26.0` (`/:version/:phone_number_id/messages`).
   - Bearer token authentication, interactive quick replies (`type: "button"`), list menus (`type: "list"`), and media upload/download (`/:version/:phone_number_id/media`).
   - Webhook handshake verification (`GET /webhook` with `hub.challenge` and `hub.verify_token`).

3. **Telegram Bot API Engine**:
   - Token-scoped routing (`/bot<token>/...` and `/file/bot<token>/...`).
   - Supports `sendMessage`, `sendVoice`, `sendAudio`, `sendPhoto`, `answerCallbackQuery`, `setWebhook`, `setMyCommands`, and `setChatMenuButton`.
   - Inline keyboards (`InlineKeyboardMarkup`) and reply keyboards (`ReplyKeyboardMarkup`).

---

## Real-Time Event Broker & SSE

`omnic` includes an in-process, lock-free event broker (`internal/service/broker.go`):

- Clients subscribe to `GET /emu/events` with optional filtering by phone number (`?number=...`) or engine (`?engine=...`).
- Broadcasts message events: `message.inbound`, `message.outbound`, `message.status`.
- Emits periodic 15-second heartbeat comments (`: ping`) to prevent intermediate proxy timeout.
- Channels are buffered and subscriber drops are handled gracefully without blocking publishers.

---

## Voice Note & Audio Pipeline

`omnic` and `omnic-handset` support end-to-end voice notes:

- **Encoding**: Recorded in OGG Opus container (`audio/ogg; codecs=opus`).
- **Media Hosting**: Uploaded through Meta Graph API or Twilio Media endpoints, stored with binary streaming support.
- **Waveform Analysis**: Computes 5-bit amplitude envelopes ($0$–$31$) across time slices for interactive visualization (`internal/ffmpeg/waveform.go`).
- **Playback**: Integrated non-blocking playback via `ffplay` inside the handset TUI.

---

## Storage Backends

| Backend | Flag | Characteristics |
| :--- | :--- | :--- |
| **In-Memory** *(default)* | Omit `--db` | High-throughput, zero-allocation storage guarded by fine-grained `sync.RWMutex` locks. Ephemeral; resets on restart. Ideal for fast unit/integration tests and CI runners. |
| **SQLite** | `--db <path>` | Embedded SQLite powered by `ncruces/go-sqlite3` (pure Go via WASM/wazero, zero CGO). Persists messages, numbers, services, and simulation rules across restarts. |
