#!/usr/bin/env python3
"""Twilio Echo Server in Python (Zero Dependencies)

Receives inbound SMS webhook from omnic and replies with:
You said, "<message>"
"""

import html
import os
import sys
from http.server import HTTPServer, BaseHTTPRequestHandler
from urllib.parse import parse_qs

# Optional Twilio SDK support if installed
try:
    from twilio.request_validator import RequestValidator
    HAS_TWILIO = True
except ImportError:
    HAS_TWILIO = False

PORT = int(os.getenv("PORT", "8000"))
HOST = os.getenv("HOST", "127.0.0.1")
AUTH_TOKEN = os.getenv("TWILIO_AUTH_TOKEN", "authtoken")
validator = RequestValidator(AUTH_TOKEN) if HAS_TWILIO else None


class EchoHandler(BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path.startswith("/api/sms/inbound"):
            content_length = int(self.headers.get("Content-Length", 0))
            post_body = self.rfile.read(content_length).decode("utf-8")
            params = parse_qs(post_body, keep_blank_values=True)
            
            # Extract flat params for signature validation and body
            flat_params = {k: v[0] for k, v in params.items()}
            sig = self.headers.get("X-Twilio-Signature")
            
            if validator and sig:
                host_hdr = self.headers.get("Host", f"{HOST}:{PORT}")
                full_url = f"http://{host_hdr}{self.path}"
                if not validator.validate(full_url, flat_params, sig):
                    self.send_response(403)
                    self.end_headers()
                    self.wfile.write(b"Invalid Twilio signature\n")
                    return

            user_body = flat_params.get("Body", "")
            reply_text = f'You said, "{user_body}"'
            xml_response = (
                '<?xml version="1.0" encoding="UTF-8"?>\n'
                '<Response>\n'
                '    <Message>\n'
                f'        <Body>{html.escape(reply_text)}</Body>\n'
                '    </Message>\n'
                '</Response>\n'
            ).encode("utf-8")

            self.send_response(200)
            self.send_header("Content-Type", "application/xml; charset=utf-8")
            self.send_header("Content-Length", str(len(xml_response)))
            self.end_headers()
            self.wfile.write(xml_response)
            return

        if self.path.startswith("/api/sms/status"):
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"status":"ok"}')
            return

        self.send_response(404)
        self.end_headers()

    def log_message(self, format, *args):
        sys.stderr.write(f"[{self.log_date_time_string()}] {format % args}\n")


def run():
    server = HTTPServer((HOST, PORT), EchoHandler)
    print(f"Python Echo Server listening on http://{HOST}:{PORT}")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()


if __name__ == "__main__":
    run()
