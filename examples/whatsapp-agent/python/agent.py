import os
import hmac
import hashlib
import json
import httpx
from fastapi import FastAPI, Request, Header, HTTPException, BackgroundTasks, Response

app = FastAPI(title="WhatsApp AI Support Agent")

WA_APP_SECRET = os.getenv("WA_APP_SECRET", "wa_secret_dev")
WA_API_BASE = os.getenv("WA_API_BASE", "http://127.0.0.1:8080")
WA_PHONE_ID = os.getenv("WA_PHONE_ID", "100000000000001")
WA_TOKEN = os.getenv("WA_TOKEN", "wa_token_dev")


def verify_whatsapp_signature(body: bytes, signature_header: str) -> bool:
    if not signature_header or not signature_header.startswith("sha256="):
        return False
    expected = "sha256=" + hmac.new(WA_APP_SECRET.encode(), body, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, signature_header)


async def send_whatsapp_reply(to: str, message_payload: dict):
    url = f"{WA_API_BASE}/v26.0/{WA_PHONE_ID}/messages"
    headers = {
        "Authorization": f"Bearer {WA_TOKEN}",
        "Content-Type": "application/json",
    }
    payload = {"messaging_product": "whatsapp", "to": to, **message_payload}
    async with httpx.AsyncClient() as client:
        await client.post(url, json=payload, headers=headers)


@app.get("/webhook")
async def verify_webhook(request: Request):
    mode = request.query_params.get("hub.mode")
    token = request.query_params.get("hub.verify_token")
    challenge = request.query_params.get("hub.challenge")
    if mode == "subscribe" and token == os.getenv("WA_VERIFY_TOKEN", "wa_verify_token"):
        return Response(content=challenge, media_type="text/plain")
    raise HTTPException(status_code=403, detail="Verification failed")


@app.post("/webhook")
async def handle_whatsapp_webhook(
    request: Request,
    background_tasks: BackgroundTasks,
    x_hub_signature_256: str = Header(None),
):
    body = await request.body()
    if not verify_whatsapp_signature(body, x_hub_signature_256):
        raise HTTPException(status_code=403, detail="Invalid HMAC-SHA256 signature")

    data = json.loads(body)
    entries = data.get("entry", [])
    for entry in entries:
        for change in entry.get("changes", []):
            val = change.get("value", {})
            for msg in val.get("messages", []):
                sender = msg.get("from")
                msg_type = msg.get("type")

                if msg_type == "text":
                    # Send interactive button reply
                    background_tasks.add_task(
                        send_whatsapp_reply,
                        sender,
                        {
                            "type": "interactive",
                            "interactive": {
                                "type": "button",
                                "body": {"text": "How can Acme Support assist you today?"},
                                "action": {
                                    "buttons": [
                                        {"type": "reply", "reply": {"id": "btn_status", "title": "Order Status"}},
                                        {"type": "reply", "reply": {"id": "btn_support", "title": "Tech Support"}},
                                    ]
                                },
                            },
                        },
                    )
                elif msg_type == "interactive":
                    btn = msg.get("interactive", {}).get("button_reply", {})
                    btn_id = btn.get("id")
                    title = btn.get("title", "")
                    background_tasks.add_task(
                        send_whatsapp_reply,
                        sender,
                        {"type": "text", "text": {"body": f"Thank you for selecting {title} ({btn_id})! An agent is reviewing."}},
                    )
                elif msg_type == "audio":
                    background_tasks.add_task(
                        send_whatsapp_reply,
                        sender,
                        {"type": "text", "text": {"body": "Voice note received! Processing audio speech-to-text..."}},
                    )

    return {"status": "ok"}
