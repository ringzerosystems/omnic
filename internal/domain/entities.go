package domain

import (
	"errors"
	"time"
)

var (
	// ErrNotFound is returned when a requested entity does not exist.
	ErrNotFound = errors.New("resource not found")

	// ErrConflict is returned when an entity violates uniqueness.
	ErrConflict = errors.New("resource conflict")
)

// MessageStatus represents the delivery state of a message.
type MessageStatus string

const (
	StatusAccepted    MessageStatus = "accepted"
	StatusQueued      MessageStatus = "queued"
	StatusSending     MessageStatus = "sending"
	StatusSent        MessageStatus = "sent"
	StatusDelivered   MessageStatus = "delivered"
	StatusUndelivered MessageStatus = "undelivered"
	StatusFailed      MessageStatus = "failed"
	StatusReceived    MessageStatus = "received"
	StatusCanceled    MessageStatus = "canceled"
	StatusScheduled   MessageStatus = "scheduled"
	StatusRead        MessageStatus = "read"
)

// MessageDirection represents the message flow direction.
type MessageDirection string

const (
	DirectionInbound       MessageDirection = "inbound"
	DirectionOutboundAPI   MessageDirection = "outbound-api"
	DirectionOutboundCall  MessageDirection = "outbound-call"
	DirectionOutboundReply MessageDirection = "outbound-reply"
)

// Capabilities represents the phone number feature flags.
type Capabilities struct {
	Voice bool
	SMS   bool
	MMS   bool
}

// Engine represents the protocol provider.
type Engine string

const (
	EngineTwilio   Engine = "twilio"
	EngineWhatsApp Engine = "whatsapp"
	EngineTelegram Engine = "telegram"
)

// InteractiveType defines WhatsApp and Telegram interactive message categories.
type InteractiveType string

const (
	InteractiveTypeButton           InteractiveType = "button"
	InteractiveTypeList             InteractiveType = "list"
	InteractiveTypeButtonReply      InteractiveType = "button_reply"
	InteractiveTypeListReply        InteractiveType = "list_reply"
	InteractiveTypeTelegramInline   InteractiveType = "tg_inline"
	InteractiveTypeTelegramReply    InteractiveType = "tg_reply"
	InteractiveTypeTelegramCallback InteractiveType = "tg_callback"
)

// ButtonItem represents an interactive quick reply button.
type ButtonItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// MenuRow represents a single option inside a list menu section.
type MenuRow struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// MenuSection represents a grouped list of menu options.
type MenuSection struct {
	Title string    `json:"title"`
	Rows  []MenuRow `json:"rows"`
}

// InteractiveContent holds structured buttons or menus.
type InteractiveContent struct {
	Type        InteractiveType `json:"type"`
	Header      string          `json:"header,omitempty"`
	Body        string          `json:"body"`
	Footer      string          `json:"footer,omitempty"`
	ActionLabel string          `json:"action_label,omitempty"`
	SelectedID  string          `json:"selected_id,omitempty"`
	Buttons     []ButtonItem    `json:"buttons,omitempty"`
	Sections    []MenuSection   `json:"sections,omitempty"`
}

// VoiceMetadata stores audio duration and voice-note properties.
type VoiceMetadata struct {
	Waveform        []int   `json:"waveform,omitempty"`
	DurationSeconds float64 `json:"duration_seconds"`
	IsVoiceNote     bool    `json:"is_voice_note"`
}

// TelegramMetadata stores Telegram-specific message attributes and keyboards.
type TelegramMetadata struct {
	CallbackQueryID string         `json:"callback_query_id,omitempty"`
	CallbackData    string         `json:"callback_data,omitempty"`
	InlineKeyboard  [][]ButtonItem `json:"inline_keyboard,omitempty"`
	ReplyKeyboard   [][]ButtonItem `json:"reply_keyboard,omitempty"`
	ChatID          int64          `json:"chat_id"`
	MessageID       int            `json:"message_id"`
	RemoveKeyboard  bool           `json:"remove_keyboard,omitempty"`
	ForceReply      bool           `json:"force_reply,omitempty"`
}

// Message represents an SMS or MMS message resource.
type Message struct {
	DateCreated         time.Time
	DateUpdated         time.Time
	DateSent            *time.Time
	SendAt              *time.Time
	Price               *string
	ErrorMessage        *string
	ErrorCode           *int
	Properties          map[string]any
	InteractiveContent  *InteractiveContent
	VoiceMetadata       *VoiceMetadata
	TelegramMetadata    *TelegramMetadata
	AccountSID          string
	PriceUnit           string
	Direction           MessageDirection
	MessagingServiceSID string
	To                  string
	From                string
	Body                string
	SID                 string
	ScheduleType        string
	StatusCallback      string
	Status              MessageStatus
	Engine              Engine
	MediaURLs           []string
	NumSegments         int
	NumMedia            int
	ValidityPeriod      int
}

// PhoneNumber represents an incoming or provisioned phone number.
type PhoneNumber struct {
	DateCreated       time.Time
	DateUpdated       time.Time
	SID               string
	AccountSID        string
	PhoneNumber       string
	FriendlyName      string
	Status            string
	SmsURL            string
	SmsMethod         string
	SmsFallbackURL    string
	SmsFallbackMethod string
	StatusCallback    string
	Capabilities      Capabilities
}

// Media represents an MMS media attachment.
type Media struct {
	DateCreated time.Time
	DateUpdated time.Time
	SID         string
	AccountSID  string
	ParentSID   string
	ContentType string
	Filename    string
	URI         string
	Data        []byte
}

// OptOutRecord tracks STOP/START compliance status for a recipient number.
type OptOutRecord struct {
	DateUpdated time.Time
	AccountSID  string
	PhoneNumber string
	OptedOut    bool
}

// MessagingService represents a Twilio Messaging Service (Copilot).
type MessagingService struct {
	DateCreated       time.Time
	DateUpdated       time.Time
	SID               string
	AccountSID        string
	FriendlyName      string
	InboundRequestURL string
	InboundMethod     string
	FallbackURL       string
	FallbackMethod    string
	StatusCallback    string
	StickySender      bool
	Geomatch          bool
}

// ServicePhoneNumber represents a phone number bound to a Messaging Service sender pool.
type ServicePhoneNumber struct {
	DateCreated    time.Time
	ServiceSID     string
	PhoneNumberSID string
	PhoneNumber    string
}

// MessageFilter defines criteria for querying messages.
type MessageFilter struct {
	DateSent *time.Time
	To       string
	From     string
	Engine   Engine
	Limit    int
	Offset   int
}

// PhoneNumberFilter defines criteria for querying phone numbers.
type PhoneNumberFilter struct {
	PhoneNumber  string
	FriendlyName string
	Limit        int
	Offset       int
}
