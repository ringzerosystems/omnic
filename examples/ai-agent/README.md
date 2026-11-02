# Reference AI Agent Examples

This directory contains reference implementations of conversational AI agent servers designed to integrate with `omnic`.

## Architecture Flow

```text
┌────────────────────────┐         Inbound SMS         ┌────────────────────────┐
│ omnic-handset (Handset)│ ──────────────────────────> │       omnic       │
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

## Running the Examples

### 1. Python (FastAPI + `twilio-python`)

```bash
cd examples/ai-agent/python
pip install -r requirements.txt
python agent.py
```

Runs on `http://127.0.0.1:8000`.

### 2. Node.js (Express + `twilio`)

```bash
cd examples/ai-agent/nodejs
npm install
npm start
```

Runs on `http://127.0.0.1:8000`.

### 3. Start `omnic` Bridge

Start the emulator configured with the upstream AI agent URL:

```bash
make build
./build/omnic \
  --twilio-upstream-url http://127.0.0.1:8000/api/sms/inbound \
  --twilio-account AC00000000000000000000000000000001:authtoken \
  --twilio-number "+14155550199@AC00000000000000000000000000000001"
```

### 4. Connect the Interactive Handset (`omnic-handset`)

Open a new terminal and launch the mobile handset TUI:

```bash
make build-handset
./build/omnic-handset --server http://127.0.0.1:8080 --from +15551234567 --to +14155550199
```

Type a message and press `Enter`. The handset sends an inbound SMS to `omnic`, which forwards a signed webhook to the AI Agent. The AI agent replies asynchronously via Twilio REST API, and the response appears in real time as a chat bubble in the handset!
