const crypto = require('crypto');
const express = require('express');

const app = express();
app.use(express.json({
  verify: (req, res, buf) => {
    req.rawBody = buf;
  }
}));

const WA_APP_SECRET = process.env.WA_APP_SECRET || 'wa_secret_dev';
const WA_API_BASE = process.env.WA_API_BASE || 'http://127.0.0.1:8080';
const WA_PHONE_ID = process.env.WA_PHONE_ID || '100000000000001';
const WA_TOKEN = process.env.WA_TOKEN || 'wa_token_dev';

function verifySignature(req) {
  const sig = req.headers['x-hub-signature-256'];
  if (!sig || !sig.startsWith('sha256=')) return false;
  const hmac = crypto.createHmac('sha256', WA_APP_SECRET);
  hmac.update(req.rawBody || '');
  const expected = 'sha256=' + hmac.digest('hex');
  return crypto.timingSafeEqual(Buffer.from(sig), Buffer.from(expected));
}

async function sendWhatsAppReply(to, messagePayload) {
  const url = `${WA_API_BASE}/v26.0/${WA_PHONE_ID}/messages`;
  const payload = { messaging_product: 'whatsapp', to, ...messagePayload };
  try {
    await fetch(url, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${WA_TOKEN}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(payload),
    });
  } catch (err) {
    console.error('Failed to send reply:', err);
  }
}

app.get('/webhook', (req, res) => {
  const mode = req.query['hub.mode'];
  const token = req.query['hub.verify_token'];
  const challenge = req.query['hub.challenge'];
  if (mode === 'subscribe' && token === (process.env.WA_VERIFY_TOKEN || 'wa_verify_token')) {
    return res.status(200).send(challenge);
  }
  return res.sendStatus(403);
});

app.post('/webhook', (req, res) => {
  if (!verifySignature(req)) {
    return res.status(403).json({ error: 'Invalid HMAC signature' });
  }

  const entries = req.body.entry || [];
  for (const entry of entries) {
    for (const change of entry.changes || []) {
      const val = change.value || {};
      for (const msg of val.messages || []) {
        const sender = msg.from;
        if (msg.type === 'text') {
          sendWhatsAppReply(sender, {
            type: 'interactive',
            interactive: {
              type: 'button',
              body: { text: 'Welcome to Acme Support! Select an option:' },
              action: {
                buttons: [
                  { type: 'reply', reply: { id: 'btn_help', title: 'Get Help' } },
                  { type: 'reply', reply: { id: 'btn_agent', title: 'Live Agent' } },
                ]
              }
            }
          });
        } else if (msg.type === 'interactive') {
          const btn = msg.interactive?.button_reply || {};
          sendWhatsAppReply(sender, {
            type: 'text',
            text: { body: `Selection received: ${btn.title} (${btn.id}). Working on it!` }
          });
        }
      }
    }
  }

  return res.status(200).json({ status: 'ok' });
});

const PORT = process.env.PORT || 8000;
app.listen(PORT, () => {
  console.log(`WhatsApp AI Agent listening on port ${PORT}`);
});
