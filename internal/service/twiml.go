package service

import (
	"encoding/xml"
	"strings"
)

// TwiMLMessage represents a <Message> verb inside TwiML <Response>.
type TwiMLMessage struct {
	To     string   `xml:"to,attr"`
	From   string   `xml:"from,attr"`
	Action string   `xml:"action,attr"`
	Method string   `xml:"method,attr"`
	Body   string   `xml:"Body"`
	Media  []string `xml:"Media"`
}

// TwiMLRedirect represents a <Redirect> verb inside TwiML <Response>.
type TwiMLRedirect struct {
	Method string `xml:"method,attr"`
	URL    string `xml:",chardata"`
}

// TwiMLResponse represents the root <Response> document.
type TwiMLResponse struct {
	Redirect *TwiMLRedirect `xml:"Redirect"`
	XMLName  xml.Name       `xml:"Response"`
	Messages []TwiMLMessage `xml:"Message"`
}

// ParseTwiML parses an XML document into a TwiMLResponse structure.
func ParseTwiML(data []byte) (*TwiMLResponse, error) {
	var resp TwiMLResponse
	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	if resp.Redirect != nil {
		resp.Redirect.URL = strings.TrimSpace(resp.Redirect.URL)
	}
	for i := range resp.Messages {
		resp.Messages[i].Body = strings.TrimSpace(resp.Messages[i].Body)
		for j := range resp.Messages[i].Media {
			resp.Messages[i].Media[j] = strings.TrimSpace(resp.Messages[i].Media[j])
		}
	}
	return &resp, nil
}
