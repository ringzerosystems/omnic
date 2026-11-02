#!/usr/bin/env node
/**
 * Twilio Echo Server in Node.js (Zero Dependencies)
 *
 * Receives inbound SMS webhook from omnic and replies with:
 * You said, "<message>"
 */

const http = require('node:http');
const querystring = require('node:querystring');

// Optional Twilio SDK support if installed
let twilio = null;
try {
  twilio = require('twilio');
} catch {
  twilio = null;
}

const PORT = parseInt(process.env.PORT || '8000', 10);
const HOST = process.env.HOST || '127.0.0.1';
const AUTH_TOKEN = process.env.TWILIO_AUTH_TOKEN || 'authtoken';

function escapeXml(unsafe) {
  return unsafe.replace(/[<>&'"]/g, (c) => {
    switch (c) {
      case '<': return '&lt;';
      case '>': return '&gt;';
      case '&': return '&amp;';
      case '\'': return '&apos;';
      case '"': return '&quot;';
      default: return c;
    }
  });
}

const server = http.createServer((req, res) => {
  if (req.method === 'POST' && req.url.startsWith('/api/sms/inbound')) {
    let raw = '';
    req.on('data', (chunk) => {
      raw += chunk;
    });

    req.on('end', () => {
      const params = querystring.parse(raw);
      const signature = req.headers['x-twilio-signature'];

      if (twilio && signature) {
        const hostHeader = req.headers.host || `${HOST}:${PORT}`;
        const fullUrl = `http://${hostHeader}${req.url}`;
        const isValid = twilio.validateRequest(AUTH_TOKEN, signature, fullUrl, params);
        if (!isValid) {
          res.writeHead(403, { 'Content-Type': 'text/plain' });
          res.end('Invalid Twilio signature\n');
          return;
        }
      }

      const userQuery = params.Body || '';
      const replyText = `You said, "${userQuery}"`;
      const xml = `<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Message>
        <Body>${escapeXml(replyText)}</Body>
    </Message>
</Response>
`;

      res.writeHead(200, {
        'Content-Type': 'application/xml; charset=utf-8',
        'Content-Length': Buffer.byteLength(xml)
      });
      res.end(xml);
    });
    return;
  }

  if (req.method === 'POST' && req.url.startsWith('/api/sms/status')) {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ status: 'ok' }));
    return;
  }

  res.writeHead(404);
  res.end('Not Found\n');
});

server.listen(PORT, HOST, () => {
  console.log(`Node.js Echo Server listening on http://${HOST}:${PORT}`);
});
