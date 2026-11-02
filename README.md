# omnic

[![RingZero Systems](https://img.shields.io/badge/Vendor-RingZero_Systems_LLC-00599C?style=for-the-badge&logo=go)](https://ringzerosystems.com)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPLv3-red.svg?style=for-the-badge)](./LICENSE)
[![Commercial License](https://img.shields.io/badge/License-Commercial-blue.svg?style=for-the-badge)](./LICENSE-COMMERCIAL.md)
[![Go Version](https://img.shields.io/badge/Go-1.27+-00ADD8.svg?style=for-the-badge&logo=go)](https://go.dev/)

A high-performance, concurrent, zero-dependency local communications emulator for **Twilio** (Programmable Messaging, Numbers, Messaging Services), **Meta WhatsApp Cloud API**, and **Telegram Bot API**.

`omnic` eliminates the cost, latency, and webhook tunneling headaches of testing against live communication providers. Point official SDKs to `localhost:8080` for instant, hermetic local development and CI testing.

---

## Key Highlights

- **Tri-Engine Protocol Parity**:
  - **Twilio**: REST API v2010 (`/2010-04-01/...`) and Messaging Services v1 (`/v1/Services`).
  - **Meta WhatsApp**: Cloud API versions `v21.0` through `v26.0` (`/:version/:phone_number_id/...`).
  - **Telegram**: Bot API (`/bot<token>/...`) with webhook delivery and secret token verification.
- **Rich Interactive Messaging & Voice**: Quick reply buttons, list menus, Telegram inline keyboards, and voice notes (`audio/ogg; codecs=opus`) with 5-bit waveform envelopes and audio playback.
- **AI Agent Webhook Bridge**: Automatic inbound webhook forwarding with authentic HMAC-SHA1/HMAC-SHA256 signatures, TwiML auto-responses, and asynchronous `StatusCallback` transitions.
- **Real-Time Streaming**: Sub-millisecond Server-Sent Events (`GET /emu/events`) with phone number and engine filtering.
- **Interactive Companion Handset (`omnic-handset`)**: Full-duplex terminal TUI emulating a user's mobile smartphone with real-time chat bubbles, delivery checkmarks, and voice recording.
- **Carrier & Failure Simulation**: Inject carrier filtering (Error 30007), unreachable destinations (30003), rate limits (HTTP 429), and webhook timeouts via CLI flags or dynamic REST rules.
- **Storage Engines**: Fast zero-allocation in-memory storage (default) or persistent pure-Go SQLite (`--db`).

---

## Documentation

Comprehensive architecture, configuration, and API documentation is available in [`docs/`](docs/):

| Document | Description |
| :--- | :--- |
| 📐 [**Architecture & Design**](docs/architecture.md) | Hexagonal layering, AI agent bridge flow, protocol engines, SSE broker, and storage. |
| ⚙️ [**Configuration & Flags**](docs/configuration.md) | Server and handset CLI flags, environment variables, `.env` file, and failure rules syntax. |
| 📖 [**API Reference**](docs/api-reference.md) | Complete endpoints for Twilio, WhatsApp, Telegram, `/emu` simulation routes, and health probes. |
| 📱 [**Conversational Handset**](docs/handset.md) | Bubble Tea TUI guide, shortcuts, voice note recording/playback, and headless mode. |
| 🛠️ [**Development & Observability**](docs/development.md) | `make` targets, QA pipeline, Docker Compose stack (Prometheus & Grafana), and pprof profiling. |

---

## Quick Start

### 1. Build and Run

```bash
# Clone the repository
git clone https://github.com/ringzerosystems/omnic.git
cd omnic

# Compile emulator and handset
make all

# Start emulator with demo numbers and development logging
./build/omnic -seed -dev
```

The emulator listens on `http://127.0.0.1:8080`, with metrics and pprof on `http://127.0.0.1:6060`.

### 2. Point Your SDKs

Override the base URL in your official client SDKs:

#### Python (`twilio-python`)

```python
from twilio.rest import Client

client = Client("AC001", "mock-auth-token")
client.http_client.api_base_url = "http://localhost:8080"

msg = client.messages.create(
    body="Hello from local dev!",
    from_="+14155550199",
    to="+14155551111"
)
print(f"Sent SID: {msg.sid}")
```

#### Node.js (`twilio-node`)

```javascript
const twilio = require('twilio');
const client = twilio('AC001', 'mock-auth-token', { accountSid: 'AC001' });

client.api.v2010.baseUrl = 'http://localhost:8080';

async function run() {
  const msg = await client.messages.create({
    body: 'Hello from local dev!',
    from: '+14155550199',
    to: '+14155551111'
  });
  console.log(`Sent SID: ${msg.sid}`);
}
run();
```

#### cURL

```bash
curl -X POST http://localhost:8080/2010-04-01/Accounts/AC001/Messages.json \
  -u AC001:mock-auth-token \
  -d "From=+14155550199" \
  -d "To=+14155551111" \
  -d "Body=Hello from cURL!"
```

---

## Connecting External AI Agents

Connect external conversational agents, bots, and LLM reasoning servers locally without ngrok:

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

1. **Inbound Forwarding**: The handset sends a message; `omnic` signs it with standard HMAC signatures and forwards it to `--twilio-upstream-url` (or WhatsApp/Telegram upstream URLs).
2. **AI Agent Reasoning**: The external server verifies the signature, processes the prompt, and replies via synchronous TwiML or asynchronous REST API calls.
3. **Real-Time Delivery**: `omnic` pushes replies instantly to the waiting handset over SSE and dispatches delivery receipts.

See [Architecture Guide](docs/architecture.md) and [`examples/`](examples/) for runnable reference implementations:

- [AI Agent Examples](examples/ai-agent/) ([Python](examples/ai-agent/python/), [Node.js](examples/ai-agent/nodejs/))
- [Echo Server Examples](examples/echo-server/) ([Python](examples/echo-server/python/), [Node.js](examples/echo-server/nodejs/))
- [WhatsApp Agent Examples](examples/whatsapp-agent/) ([Python](examples/whatsapp-agent/python/), [Node.js](examples/whatsapp-agent/nodejs/))

---

## Interactive Companion Handset (`omnic-handset`)

Launch the companion mobile handset TUI to interact with your services in real time:

```bash
# Launch interactive tri-engine mobile handset
./build/omnic-handset --engine all --from +14155551111 --to +14155550199

# Send a one-shot message via CLI and exit
./build/omnic-handset --engine all --from +14155551111 --to +14155550199 --body "Where is my order?"
```

See the [Handset Guide](docs/handset.md) for keyboard shortcuts, list menus, and voice recording controls.

---

## Development & Verification

All workflows are driven through `make`:

```bash
make help          # List all available targets
make all           # Compile both emulator server and handset
make build         # Compile static binary -> build/omnic
make build-handset # Compile handset binary -> build/omnic-handset
make test          # Run test suite with race detector (CGO_ENABLED=1 go test -race)
make qa            # Full hermetic static quality gate (fix -> fmt -> vet -> lint -> nilaway -> arch-go -> loc -> fieldalignment)
make bench         # Run benchmarks -> build/bench/new.txt
make profile       # Profile server under load -> build/profile/summary.txt
```

See [Development & Observability](docs/development.md) for details on the local Prometheus & Grafana stack (`make infra-up`).

---

## License & Commercial Use

Copyright &copy; 2026 Mark Horila / RingZero Systems LLC.

`omnic` is dual-licensed:

1. **Open Source ([AGPL-3.0](./LICENSE)):** Free for open-source development, academic evaluation, and workflows that comply with the GNU Affero General Public License v3.0.
2. **Commercial License ([LICENSE-COMMERCIAL.md](./LICENSE-COMMERCIAL.md)):** Organizations deploying, embedding, or modifying `omnic` in proprietary commercial environments or hosted internal developer platforms without AGPL-3.0 copyleft obligations may obtain a commercial license from **RingZero Systems LLC**.

For commercial licensing, enterprise pilots, or custom AST/compiler integration sprints, contact:

- **Web:** [https://ringzerosystems.com](https://ringzerosystems.com)
- **Direct:** `licensing@ringzerosystems.com`
