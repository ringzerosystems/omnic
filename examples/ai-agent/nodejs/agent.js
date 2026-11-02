const express = require('express');
const twilio = require('twilio');

const app = express();
app.use(express.urlencoded({ extended: false }));

const accountSid = process.env.TWILIO_ACCOUNT_SID || 'AC00000000000000000000000000000001';
const authToken = process.env.TWILIO_AUTH_TOKEN || 'authtoken';
const client = twilio(accountSid, authToken, {
  apiBase: process.env.TWILIO_API_BASE || 'http://127.0.0.1:8080'
});

const agentNumber = process.env.TWILIO_PHONE_NUMBER || '+14155550199';

app.post('/api/sms/inbound', (req, res) => {
  const userNumber = req.body.From;
  const userQuery = req.body.Body || '';

  // Return immediate 200 OK TwiML to acknowledge inbound webhook
  res.type('text/xml').send('<Response/>');

  // Asynchronous Agentic Loop: simulate LLM latency and send reply via Twilio REST API
  setTimeout(async () => {
    try {
      await client.messages.create({
        to: userNumber,
        from: agentNumber,
        body: `AI Assistant: Processed "${userQuery}". All tasks complete.`,
        statusCallback: 'http://127.0.0.1:8000/api/sms/status'
      });
    } catch (err) {
      console.error('Failed to send async reply:', err);
    }
  }, 1500);
});

app.post('/api/sms/status', (req, res) => {
  console.log(`Status update for ${req.body.MessageSid}: ${req.body.MessageStatus}`);
  res.status(200).send('OK');
});

const PORT = process.env.PORT || 8000;
app.listen(PORT, () => {
  console.log(`Node.js AI Agent listening on port ${PORT}`);
});
