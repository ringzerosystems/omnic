# Meta WhatsApp Business Platform AI Agent Examples

This directory contains reference implementations of conversational AI agents integrating with `omnic`'s Meta WhatsApp Business Platform (Cloud API v21.0/v26.0) emulation layer.

## Architecture

```text
┌────────────────────────┐      Inbound WhatsApp Message      ┌────────────────────────┐
│ omnic-handset (Handset)│ ─────────────────────────────────> │       omnic       │
│      14155551111       │                                    │         :8080          │
└────────────────────────┘                                    └────────────────────────┘
            ▲                                                              │
            │                                                              │ Signed Webhook
            │ SSE Stream (message.outbound)                                │ X-Hub-Signature-256
            │ GET /emu/events?number=...                                   ▼
            │                                                 ┌────────────────────────┐
            └──────────────────────────────────────────────── │ WhatsApp AI Agent      │
                    Outbound Graph API Reply                  │         :8000          │
                 POST /v26.0/:phone_id/messages               └────────────────────────┘
```

1. **Inbound Webhook Verification (`X-Hub-Signature-256`)**: The emulator signs all incoming webhooks with HMAC-SHA256 using the configured app secret.
2. **Interactive Quick Reply Buttons**: The agent sends interactive options (`button` type); the handset renders clickable/numbered pills (`1`, `2`, `3`).
3. **Voice Notes**: The handset records and uploads voice notes (`audio/ogg; codecs=opus`) to the emulator, which dispatches audio messages to the agent.

## Quick Start

### 1. Start `omnic` in Multi-Engine Mode

```bash
./build/omnic \
  --engines both \
  --whatsapp-upstream-url http://127.0.0.1:8000/webhook \
  --dev
```

### 2. Start the AI Agent

#### Python (FastAPI)

```bash
cd python
pip install -r requirements.txt
uvicorn agent:app --port 8000
```

#### Node.js (Express)

```bash
cd nodejs
npm install
npm start
```

### 3. Start `omnic-handset` Handset

```bash
./build/omnic-handset --engine whatsapp --from 14155551111 --to 100000000000001
```

Type any text (e.g. `Hello`) and press Enter. The agent will respond with interactive quick reply buttons!
