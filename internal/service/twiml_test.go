package service_test

import (
	"testing"

	"github.com/ringzerosystems/omnic/internal/service"
)

func TestParseTwiML(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Message to="+14155551111" from="+14155552222">
        <Body>Hello from TwiML reply!</Body>
        <Media>https://example.com/image.png</Media>
    </Message>
</Response>`

	resp, err := service.ParseTwiML([]byte(xmlData))
	if err != nil {
		t.Fatalf("ParseTwiML failed: %v", err)
	}

	if len(resp.Messages) != 1 {
		t.Fatalf("expected 1 message in TwiML, got %d", len(resp.Messages))
	}
	msg := resp.Messages[0]
	if msg.To != "+14155551111" || msg.From != "+14155552222" {
		t.Errorf("got To=%s From=%s", msg.To, msg.From)
	}
	if msg.Body != "Hello from TwiML reply!" {
		t.Errorf("got Body=%s", msg.Body)
	}
	if len(msg.Media) != 1 || msg.Media[0] != "https://example.com/image.png" {
		t.Errorf("got Media=%v", msg.Media)
	}
}

func TestParseTwiMLRedirect(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Redirect method="POST">https://example.com/next.xml</Redirect>
</Response>`

	resp, err := service.ParseTwiML([]byte(xmlData))
	if err != nil {
		t.Fatalf("ParseTwiML failed: %v", err)
	}
	if resp.Redirect == nil || resp.Redirect.URL != "https://example.com/next.xml" {
		t.Errorf("got Redirect=%v, want https://example.com/next.xml", resp.Redirect)
	}
}
