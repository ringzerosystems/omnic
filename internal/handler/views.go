package handler

import (
	"encoding/json"
	"fmt"
	"maps"
	"strconv"
	"time"

	"github.com/ringzerosystems/omnic/internal/domain"
)

// SubresourceURIs models Twilio message subresource links.
type SubresourceURIs struct {
	Media    string `json:"media"`
	Feedback string `json:"feedback"`
}

// MessageView is a strongly-typed view model for Twilio message JSON serialization.
type MessageView struct {
	DateSent            *string                    `json:"date_sent"`
	Price               *string                    `json:"price"`
	ErrorMessage        *string                    `json:"error_message"`
	ErrorCode           *int                       `json:"error_code"`
	MessagingServiceSID *string                    `json:"messaging_service_sid"`
	InteractiveContent  *domain.InteractiveContent `json:"interactive_content,omitempty"`
	VoiceMetadata       *domain.VoiceMetadata      `json:"voice_metadata,omitempty"`
	TelegramMetadata    *domain.TelegramMetadata   `json:"telegram_metadata,omitempty"`
	extra               map[string]any             `json:"-"`
	DateCreated         string                     `json:"date_created"`
	DateUpdated         string                     `json:"date_updated"`
	To                  string                     `json:"to"`
	From                string                     `json:"from"`
	Body                string                     `json:"body"`
	Status              string                     `json:"status"`
	NumSegments         string                     `json:"num_segments"`
	NumMedia            string                     `json:"num_media"`
	Direction           string                     `json:"direction"`
	APIVersion          string                     `json:"api_version"`
	PriceUnit           string                     `json:"price_unit"`
	URI                 string                     `json:"uri"`
	SID                 string                     `json:"sid"`
	AccountSID          string                     `json:"account_sid"`
	Engine              string                     `json:"engine,omitempty"`
	SubresourceURIs     SubresourceURIs            `json:"subresource_uris"`
}

// MarshalJSON preserves custom properties on MessageView with a single serialization pass.
func (v MessageView) MarshalJSON() ([]byte, error) {
	type Alias MessageView
	if len(v.extra) == 0 {
		return json.Marshal(Alias(v))
	}
	m := make(map[string]any, 24+len(v.extra))
	m["sid"] = v.SID
	m["account_sid"] = v.AccountSID
	m["to"] = v.To
	m["from"] = v.From
	m["body"] = v.Body
	m["status"] = v.Status
	m["num_segments"] = v.NumSegments
	m["num_media"] = v.NumMedia
	m["direction"] = v.Direction
	m["api_version"] = v.APIVersion
	m["price"] = v.Price
	m["price_unit"] = v.PriceUnit
	m["error_code"] = v.ErrorCode
	m["error_message"] = v.ErrorMessage
	m["uri"] = v.URI
	m["date_created"] = v.DateCreated
	m["date_updated"] = v.DateUpdated
	m["date_sent"] = v.DateSent
	m["messaging_service_sid"] = v.MessagingServiceSID
	m["subresource_uris"] = v.SubresourceURIs
	if v.Engine != "" {
		m["engine"] = v.Engine
	}
	if v.InteractiveContent != nil {
		m["interactive_content"] = v.InteractiveContent
	}
	if v.VoiceMetadata != nil {
		m["voice_metadata"] = v.VoiceMetadata
	}
	if v.TelegramMetadata != nil {
		m["telegram_metadata"] = v.TelegramMetadata
	}
	maps.Copy(m, v.extra)
	return json.Marshal(m)
}

// NewMessageView builds a MessageView from a domain Message.
func NewMessageView(msg *domain.Message) MessageView {
	var msgSvcSID *string
	if msg.MessagingServiceSID != "" {
		s := msg.MessagingServiceSID
		msgSvcSID = &s
	}

	return MessageView{
		DateSent:            formatDate(msg.DateSent),
		Price:               msg.Price,
		ErrorMessage:        msg.ErrorMessage,
		ErrorCode:           msg.ErrorCode,
		MessagingServiceSID: msgSvcSID,
		extra:               msg.Properties,
		DateCreated:         msg.DateCreated.UTC().Format(time.RFC1123Z),
		DateUpdated:         msg.DateUpdated.UTC().Format(time.RFC1123Z),
		To:                  msg.To,
		From:                msg.From,
		Body:                msg.Body,
		Status:              string(msg.Status),
		NumSegments:         strconv.Itoa(msg.NumSegments),
		NumMedia:            strconv.Itoa(msg.NumMedia),
		Direction:           string(msg.Direction),
		APIVersion:          "2010-04-01",
		PriceUnit:           msg.PriceUnit,
		URI:                 fmt.Sprintf("/2010-04-01/Accounts/%s/Messages/%s.json", msg.AccountSID, msg.SID),
		SID:                 msg.SID,
		AccountSID:          msg.AccountSID,
		Engine:              string(msg.Engine),
		InteractiveContent:  msg.InteractiveContent,
		VoiceMetadata:       msg.VoiceMetadata,
		TelegramMetadata:    msg.TelegramMetadata,
		SubresourceURIs: SubresourceURIs{
			Media:    fmt.Sprintf("/2010-04-01/Accounts/%s/Messages/%s/Media.json", msg.AccountSID, msg.SID),
			Feedback: fmt.Sprintf("/2010-04-01/Accounts/%s/Messages/%s/Feedback.json", msg.AccountSID, msg.SID),
		},
	}
}

// PhoneNumberView is a strongly-typed view model for Twilio phone number JSON serialization.
type PhoneNumberView struct {
	DateCreated       string              `json:"date_created"`
	DateUpdated       string              `json:"date_updated"`
	SID               string              `json:"sid"`
	AccountSID        string              `json:"account_sid"`
	PhoneNumber       string              `json:"phone_number"`
	FriendlyName      string              `json:"friendly_name"`
	Status            string              `json:"status"`
	SmsURL            string              `json:"sms_url"`
	SmsMethod         string              `json:"sms_method"`
	SmsFallbackURL    string              `json:"sms_fallback_url"`
	SmsFallbackMethod string              `json:"sms_fallback_method"`
	StatusCallback    string              `json:"status_callback"`
	URI               string              `json:"uri"`
	Capabilities      domain.Capabilities `json:"capabilities"`
}

// NewPhoneNumberView builds a PhoneNumberView from a domain PhoneNumber.
func NewPhoneNumberView(pn *domain.PhoneNumber) PhoneNumberView {
	return PhoneNumberView{
		DateCreated:       pn.DateCreated.UTC().Format(time.RFC1123Z),
		DateUpdated:       pn.DateUpdated.UTC().Format(time.RFC1123Z),
		SID:               pn.SID,
		AccountSID:        pn.AccountSID,
		PhoneNumber:       pn.PhoneNumber,
		FriendlyName:      pn.FriendlyName,
		Status:            pn.Status,
		SmsURL:            pn.SmsURL,
		SmsMethod:         pn.SmsMethod,
		SmsFallbackURL:    pn.SmsFallbackURL,
		SmsFallbackMethod: pn.SmsFallbackMethod,
		StatusCallback:    pn.StatusCallback,
		URI:               fmt.Sprintf("/2010-04-01/Accounts/%s/IncomingPhoneNumbers/%s.json", pn.AccountSID, pn.SID),
		Capabilities:      pn.Capabilities,
	}
}

// MediaView is a strongly-typed view model for Twilio media JSON serialization.
type MediaView struct {
	DateCreated string `json:"date_created"`
	DateUpdated string `json:"date_updated"`
	SID         string `json:"sid"`
	AccountSID  string `json:"account_sid"`
	ParentSID   string `json:"parent_sid"`
	ContentType string `json:"content_type"`
	URI         string `json:"uri"`
}

// NewMediaView builds a MediaView from a domain Media.
func NewMediaView(media *domain.Media) MediaView {
	return MediaView{
		DateCreated: media.DateCreated.UTC().Format(time.RFC1123Z),
		DateUpdated: media.DateUpdated.UTC().Format(time.RFC1123Z),
		SID:         media.SID,
		AccountSID:  media.AccountSID,
		ParentSID:   media.ParentSID,
		ContentType: media.ContentType,
		URI:         fmt.Sprintf("/2010-04-01/Accounts/%s/Messages/%s/Media/%s.json", media.AccountSID, media.ParentSID, media.SID),
	}
}

// ServiceView is a strongly-typed view model for Twilio Messaging Service JSON serialization.
type ServiceView struct {
	DateCreated       string `json:"date_created"`
	DateUpdated       string `json:"date_updated"`
	SID               string `json:"sid"`
	AccountSID        string `json:"account_sid"`
	FriendlyName      string `json:"friendly_name"`
	InboundRequestURL string `json:"inbound_request_url"`
	InboundMethod     string `json:"inbound_method"`
	FallbackURL       string `json:"fallback_url"`
	FallbackMethod    string `json:"fallback_method"`
	StatusCallback    string `json:"status_callback"`
	URL               string `json:"url"`
	StickySender      bool   `json:"sticky_sender"`
	Geomatch          bool   `json:"geomatch"`
}

// NewServiceView builds a ServiceView from a domain MessagingService with RFC1123Z dates.
func NewServiceView(s *domain.MessagingService) ServiceView {
	return ServiceView{
		DateCreated:       s.DateCreated.UTC().Format(time.RFC1123Z),
		DateUpdated:       s.DateUpdated.UTC().Format(time.RFC1123Z),
		SID:               s.SID,
		AccountSID:        s.AccountSID,
		FriendlyName:      s.FriendlyName,
		InboundRequestURL: s.InboundRequestURL,
		InboundMethod:     s.InboundMethod,
		FallbackURL:       s.FallbackURL,
		FallbackMethod:    s.FallbackMethod,
		StatusCallback:    s.StatusCallback,
		URL:               fmt.Sprintf("/v1/Services/%s", s.SID),
		StickySender:      s.StickySender,
		Geomatch:          s.Geomatch,
	}
}

// MessageListPageView represents a paginated message list response.
type MessageListPageView struct {
	PreviousPageURI *string       `json:"previous_page_uri"`
	NextPageURI     *string       `json:"next_page_uri"`
	FirstPageURI    string        `json:"first_page_uri"`
	URI             string        `json:"uri"`
	Messages        []MessageView `json:"messages"`
	Page            int           `json:"page"`
	PageSize        int           `json:"page_size"`
	Total           int           `json:"total"`
	Start           int           `json:"start"`
	End             int           `json:"end"`
}

// PhoneNumberListPageView represents a paginated phone number list response.
type PhoneNumberListPageView struct {
	PreviousPageURI      *string           `json:"previous_page_uri"`
	NextPageURI          *string           `json:"next_page_uri"`
	FirstPageURI         string            `json:"first_page_uri"`
	URI                  string            `json:"uri"`
	IncomingPhoneNumbers []PhoneNumberView `json:"incoming_phone_numbers"`
	Page                 int               `json:"page"`
	PageSize             int               `json:"page_size"`
	Total                int               `json:"total"`
	Start                int               `json:"start"`
	End                  int               `json:"end"`
}

// MediaListPageView represents a media list response.
type MediaListPageView struct {
	URI       string      `json:"uri"`
	MediaList []MediaView `json:"media_list"`
}

// ServiceListPageView represents a messaging services list response.
type ServiceListPageView struct {
	Services []ServiceView `json:"services"`
	Total    int           `json:"total"`
}
