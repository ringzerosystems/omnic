# Reference Echo Server Examples

This directory contains reference implementations of SMS echo servers designed to integrate with `omnic`.

When a user sends an SMS message such as:
```text
hello, how are you?
```

The echo server replies:
```text
You said, "hello, how are you?"
```

## Architecture Flow

The echo server supports two response modes:

### 1. Synchronous TwiML Response (Default)

Twilio forwards the incoming message webhook, and the server returns immediate TwiML XML (`<Response><Message><Body>...</Body></Message></Response>`):

```text
┌────────────────────────┐         Inbound SMS         ┌────────────────────────┐
│ omnic-handset (Handset)│ ──────────────────────────> │       omnic       │
│      +15551234567      │                             │         :8080          │
└────────────────────────┘                             └────────────────────────┘
            ▲                                                       │
            │                                                       │ Signed Webhook
            │ SSE Stream                                            │ POST /api/sms/inbound
            │ (Delivers Outbound Reply)                             ▼
            │                                          ┌────────────────────────┐
            └───────────────────────────────────────── │      Echo Server       │
                        Synchronous TwiML              │         :8000          │
                   <Response><Message><Body>           └────────────────────────┘
```

### 2. Asynchronous REST API Reply (`REPLY_MODE=rest`)

The webhook returns HTTP 200 (`<Response/>`) and uses the Twilio REST API SDK (`client.messages.create`) to dispatch an outbound message back through `omnic`:

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
            └───────────────────────────────────────── │      Echo Server       │
                      Outbound REST API Reply          │         :8000          │
                   POST /.../Messages.json             └────────────────────────┘
```

---

## Running the Examples

Both echo servers are written using standard library components with **zero external dependencies required**. You can start either one immediately without `pip install` or `npm install`!

### 1. Python (Zero Dependencies / Standard Library)

```bash
python3 examples/echo-server/python/echo.py
```

Runs on `http://127.0.0.1:8000`.

*(Optional: if `twilio` is installed in your python environment, cryptographic HMAC signature verification is automatically enabled).*

### 2. Node.js (Zero Dependencies / Built-in `node:http`)

```bash
node examples/echo-server/nodejs/echo.js
```

Runs on `http://127.0.0.1:8000`.

*(Optional: if `twilio` is installed in node_modules, cryptographic HMAC signature verification is automatically enabled).*

---

## Running with `omnic` and `omnic-handset`

### 1. Start `omnic` Bridge

Compile and start the emulator server configured with the echo server webhook:

```bash
make build
./build/omnic \
  --twilio-upstream-url http://127.0.0.1:8000/api/sms/inbound \
  --twilio-account AC00000000000000000000000000000001:authtoken \
  --twilio-number "+14155550199@AC00000000000000000000000000000001"
```

### 2. Interactive Handset (`omnic-handset`)

Open a new terminal and run the handset client:

```bash
make build-handset
./build/omnic-handset --server http://127.0.0.1:8080 --from +15551234567 --to +14155550199
```

Type `hello, how are you?` and press `Enter`. The echo server immediately replies:
```text
You said, "hello, how are you?"
```

### 3. One-Shot CLI Send

You can also send a test message directly from the command line:

```bash
./build/omnic-handset \
  --server http://127.0.0.1:8080 \
  --from +15551234567 \
  --to +14155550199 \
  --body "hello, how are you?"
```

Output:
```text
Sent SMS to +14155550199: hello, how are you?
Received TwiML Reply: You said, "hello, how are you?"
```
