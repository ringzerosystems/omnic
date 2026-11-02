# Omnic API Reference

Complete catalog of wire protocols, REST routes, simulation endpoints, and streaming interfaces supported by `omnic`.

---

## Twilio REST API v2010

All Twilio endpoints accept standard HTTP Basic Auth (`AccountSid:AuthToken`) and support `.json`, `.xml`, or extensionless URLs.

### Messages

| Method | Path | Description |
| :--- | :--- | :--- |
| `POST` | `/2010-04-01/Accounts/{AccountSid}/Messages.json` | Send an outbound SMS or MMS. Accepts `From`, `To`, `Body`, `MediaUrl`, `StatusCallback`, and `MessagingServiceSid`. |
| `GET` | `/2010-04-01/Accounts/{AccountSid}/Messages.json` | List sent and received messages with query pagination and filtering (`To`, `From`, `DateSent`). |
| `GET` | `/2010-04-01/Accounts/{AccountSid}/Messages/{Sid}.json` | Retrieve details and status of a specific message SID. |
| `POST` | `/2010-04-01/Accounts/{AccountSid}/Messages/{Sid}.json` | Update message record (cancel scheduled sends or redact body). |
| `DELETE`| `/2010-04-01/Accounts/{AccountSid}/Messages/{Sid}.json` | Delete a message record from storage. |

### Media Attachments

| Method | Path | Description |
| :--- | :--- | :--- |
| `GET` | `/2010-04-01/Accounts/{AccountSid}/Messages/{Sid}/Media.json` | List all media attachments associated with an MMS message. |
| `GET` | `/2010-04-01/Accounts/{AccountSid}/Messages/{Sid}/Media/{MediaSid}.json` | Retrieve metadata for a specific media SID. |
| `GET` | `/2010-04-01/Accounts/{AccountSid}/Messages/{Sid}/Media/{MediaSid}` | Download the raw binary media stream. |

### Phone Numbers & Provisioning

| Method | Path | Description |
| :--- | :--- | :--- |
| `GET` | `/2010-04-01/Accounts/{AccountSid}/AvailablePhoneNumbers/{CountryCode}/Local.json` | Search available phone numbers for provisioning. |
| `POST` | `/2010-04-01/Accounts/{AccountSid}/IncomingPhoneNumbers.json` | Provision a new phone number into the account. |
| `GET` | `/2010-04-01/Accounts/{AccountSid}/IncomingPhoneNumbers.json` | List all provisioned numbers under the account. |
| `GET` | `/2010-04-01/Accounts/{AccountSid}/IncomingPhoneNumbers/{Sid}.json` | Fetch configuration and webhook URLs for a provisioned number. |
| `POST` | `/2010-04-01/Accounts/{AccountSid}/IncomingPhoneNumbers/{Sid}.json` | Update number settings (e.g. `SmsUrl`, `SmsMethod`, `StatusCallback`). |
| `DELETE`| `/2010-04-01/Accounts/{AccountSid}/IncomingPhoneNumbers/{Sid}.json` | Release (deprovision) a phone number. |

---

## Twilio Messaging v1

Endpoints for managing Messaging Services and phone number pools.

| Method | Path | Description |
| :--- | :--- | :--- |
| `POST` | `/v1/Services` | Create a new Messaging Service pool (`FriendlyName`, `InboundRequestUrl`, `StatusCallback`). |
| `GET` | `/v1/Services` | List all configured Messaging Services. |
| `GET` | `/v1/Services/{ServiceSid}` | Retrieve details for a specific Messaging Service SID. |
| `POST` | `/v1/Services/{ServiceSid}/PhoneNumbers` | Associate a provisioned `PhoneNumberSid` with the service pool. |
| `GET` | `/v1/Services/{ServiceSid}/PhoneNumbers` | List all phone numbers belonging to the sender pool. |
| `DELETE`| `/v1/Services/{ServiceSid}/PhoneNumbers/{PhoneNumberSid}` | Remove a phone number from the service pool. |

---

## Meta WhatsApp Cloud API (v21.0 & v26.0)

Dynamic routing supporting versions `v21.0` through `v26.0` (`/:version/:phone_number_id/...`). Authenticated via `Authorization: Bearer <token>`.

| Method | Endpoint Pattern | Description |
| :--- | :--- | :--- |
| `POST` | `/:version/:phone_number_id/messages` | Send an outbound WhatsApp message. Supports text, audio/voice note, quick reply buttons (`type: "button"`), and interactive lists (`type: "list"`). |
| `POST` | `/:version/:phone_number_id/media` | Upload binary media asset (voice note, image) via `multipart/form-data`. Returns `id`. |
| `GET` | `/:version/:media_id` | Retrieve media metadata including download URL and content type. |
| `GET` | `/:version/media/:media_id/download` | Download raw binary media content. |
| `GET` | `/webhook` | Webhook verification handshake (`hub.mode=subscribe`, `hub.challenge`, `hub.verify_token`). |
| `POST` | `/webhook` | Inbound webhook delivery endpoint signed with `X-Hub-Signature-256`. |

---

## Telegram Bot API

Token-scoped routing matching Telegram's Bot API structure (`/bot<token>/...` and `/file/bot<token>/...`).

| Method | Endpoint Pattern | Description |
| :--- | :--- | :--- |
| `GET/POST` | `/bot<token>/getMe` | Returns bot identity and capabilities. |
| `POST` | `/bot<token>/sendMessage` | Send text message with support for `InlineKeyboardMarkup` or `ReplyKeyboardMarkup`. |
| `POST` | `/bot<token>/sendVoice` | Send PTT voice note (`audio/ogg; codecs=opus`) with duration metadata. |
| `POST` | `/bot<token>/sendAudio` | Send audio track with title and performer metadata. |
| `POST` | `/bot<token>/sendPhoto` | Send photo attachment. |
| `POST` | `/bot<token>/answerCallbackQuery` | Acknowledge inline keyboard button clicks. |
| `POST` | `/bot<token>/setWebhook` | Register upstream webhook URL and secret token (`secret_token`). |
| `POST` | `/bot<token>/deleteWebhook` | Deregister the active webhook. |
| `GET/POST` | `/bot<token>/getWebhookInfo` | Inspect active webhook status and configuration. |
| `POST` | `/bot<token>/setMyCommands` | Register bot slash commands (`/start`, `/help`, etc.). |
| `GET/POST` | `/bot<token>/getMyCommands` | Fetch registered slash commands. |
| `POST` | `/bot<token>/setChatMenuButton` | Configure the chat menu button. |
| `GET/POST` | `/bot<token>/getChatMenuButton` | Retrieve active chat menu button. |
| `GET/POST` | `/bot<token>/getFile` | Resolve `file_id` to `file_path` for binary media downloading. |
| `GET` | `/file/bot<token>/*` | Download binary media files (voice notes, audio, images). |

---

## Emulator & SSE Simulation Endpoints (`/emu`)

Special emulator routes for triggering inbound events, inspecting state, and streaming SSE events:

| Method | Path | Payload | Description |
| :--- | :--- | :--- | :--- |
| `POST` | `/emu/inbound/sms` | `{"from": "...", "to": "...", "body": "..."}` | Simulates an inbound SMS from a handset. Dispatches upstream webhooks and evaluates synchronous TwiML replies. |
| `POST` | `/emu/inbound/whatsapp` | `{"from": "...", "to": "...", "type": "...", ...}` | Simulates inbound WhatsApp message, interactive button click, or voice note. Dispatches signed webhook. |
| `POST` | `/emu/inbound/telegram` | `{"from_id": ..., "chat_id": ..., "text": "..."}` | Simulates inbound Telegram message, callback query, or voice note. Dispatches update with secret token header. |
| `GET` | `/emu/messages` | `?account_sid=...&engine=...&to=...&from=...` | Retrieves conversation messages across Twilio, WhatsApp, and Telegram for UI hydration. |
| `GET` | `/emu/events` | `?number=+15551234567` or `?engine=telegram` | Real-time Server-Sent Events (SSE) stream pushing `message.inbound`, `message.outbound`, `message.status`, and 15s heartbeats. |
| `POST` | `/emu/numbers/seed` | N/A | Seeds default test phone numbers and accounts. |
| `GET` | `/emu/simulate/rules` | N/A | Lists active fault injection simulation rules. |
| `POST` | `/emu/simulate/rules` | `{"rules": [...]}` | Registers or replaces dynamic fault injection rules. |
| `DELETE`| `/emu/simulate/rules` | N/A | Clears all dynamic fault injection rules. |

---

## Health & Kubernetes Probes

Standard endpoints for container orchestration and liveness checks:

| Probe | Endpoint | Status | Output |
| :--- | :--- | :--- | :--- |
| **Health** | `GET /healthz` | `200 OK` | `ok` (or `{"status": "ok"}` with JSON accept header) |
| **Liveness** | `GET /livez` | `200 OK` | `ok` |
| **Readiness** | `GET /readyz` | `200 OK` | `ok` |
| **Startup** | `GET /startupz` | `200 OK` | `ok` |
