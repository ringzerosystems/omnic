# Interactive Conversational Handset (`omnic-handset`)

`omnic-handset` is an interactive companion terminal handset client built with Charm's Bubble Tea v2 framework. It emulates a user's mobile smartphone with real-time SSE message streaming, multi-engine routing, interactive quick replies, and voice note recording/playback.

---

## Handset TUI Overview

```text
┌────────────────────────────────────────────────────────────────────────┐
│ MOBILE HANDSET (+14155551111)  [⚡ TRI-ENGINE]                 ● ONLINE │
│ Connected: http://localhost:8080                                       │
│ Recipient: +14155550199 (AI Agent)                                     │
├────────────────────────────────────────────────────────────────────────┤
│                                                                        │
│                                      [14:02] You [💬 WhatsApp] ✓✓ Read │
│                                      ┌───────────────────────────────┐ │
│                                      │ Schedule a demo               │ │
│                                      └───────────────────────────────┘ │
│                                                                        │
│ [14:03] 🤖 AI Support Agent: [💬 WhatsApp]                             │
│ ┌────────────────────────────────────────────────────────────────────┐ │
│ │ Please select a preferred meeting time:                            │ │
│ │                                                                    │ │
│ │ [ 1: 10:00 AM Tomorrow ]   [ 2: 2:00 PM Tomorrow ]   [ 3: Other ]  │ │
│ └────────────────────────────────────────────────────────────────────┘ │
│                                                                        │
│ [14:04] 🤖 AI Support Agent: [💬 WhatsApp]                             │
│ ┌────────────────────────────────────────────────────────────────────┐ │
│ │ 🎤 Voice Note (0:14)                                               │ │
│ │ [▶ Space: Play]  ılılı|lılı|lı|lılı|lılı                           │ │
│ └────────────────────────────────────────────────────────────────────┘ │
│                                                                        │
├────────────────────────────────────────────────────────────────────────┤
│ Send via: [ 💬 WhatsApp ] (Tab to cycle)                               │
│ ❯ Confirming for 2:00 PM█                                              │
├────────────────────────────────────────────────────────────────────────┤
│ Status: Ready                                                          │
│ Enter Send  •  Tab Target  •  V Voice  •  Space Play  •  1/2/3 Button  │
│ M Menu  •  ↑/↓ Scroll  •  Esc Quit                                     │
└────────────────────────────────────────────────────────────────────────┘
```

---

## Keyboard Shortcuts & Controls

| Key | Action | Description |
| :--- | :--- | :--- |
| `Enter` | Send Message | Dispatches the current typed input to the active send target. |
| `Tab` / `Ctrl+S` | Cycle Send Target | Swaps active protocol: `[ 💬 WhatsApp ]`, `[ 📱 Twilio ]`, `[ ✈️ Telegram ]`, or `[ ⚡ All (Broadcast) ]`. |
| `1` – `9` | Quick Reply Button | Instantly selects and dispatches interactive button pills or Telegram inline callbacks. |
| `M` | Toggle List Menu | Opens/closes interactive modal drawer for WhatsApp list menus. Navigate with `↑/↓`, select with `Enter`. |
| `Space` | Play / Pause Voice | Starts or stops non-blocking playback of the highlighted voice note via `ffplay`. |
| `V` / `Ctrl+R` | Record Voice Note | Toggles audio recording HUD with live elapsed timer and animated VU meter. Press `Enter` to send, `Esc` to discard. |
| `↑` / `↓` / `PgUp` / `PgDn` | Scroll Chat | Scrolls message history viewport. |
| `Esc` / `Ctrl+C` | Quit / Dismiss | Dismisses active menus or exits the handset. |

---

## Handset Modes

### 1. Interactive TUI Mode (Default)

Launch the interactive chat interface connected to `omnic`:

```bash
# Connect with tri-engine support
./build/omnic-handset --engine all --from +14155551111 --to +14155550199

# Connect specifically for WhatsApp testing
./build/omnic-handset --engine whatsapp --from 14155551111 --to 100000000000001

# Connect specifically for Telegram testing
./build/omnic-handset --engine telegram --telegram-chat-id 14155551111
```

### 2. Audio Device Discovery

Inspect system microphone and audio playback devices for voice note recording:

```bash
./build/omnic-handset -list-audio-devices
```

Use the discovered device names or indices with `--audio-in` and `--audio-out`:

```bash
./build/omnic-handset --audio-in "MacBook Pro Microphone" --audio-out "default"
```

### 3. Scriptable One-Shot CLI Send

Send a message directly from the command line without launching the TUI:

```bash
# Send an SMS and receive TwiML reply
./build/omnic-handset \
  --server http://127.0.0.1:8080 \
  --from +15551234567 \
  --to +14155550199 \
  --body "Hello from script!"
```

### 4. Headless Streaming Mode

Run as a headless background listener printing real-time SSE events to stdout (useful for CI or integration test assertion):

```bash
./build/omnic-handset --server http://127.0.0.1:8080 --from +15551234567 --headless
```
