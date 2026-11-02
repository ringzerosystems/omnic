package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/repository"
)

// DB wraps an SQLite database connection and provides unified repository implementations.
type DB struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS phone_numbers (
    sid TEXT PRIMARY KEY,
    account_sid TEXT NOT NULL,
    phone_number TEXT NOT NULL UNIQUE,
    friendly_name TEXT NOT NULL,
    status TEXT NOT NULL,
    sms_url TEXT NOT NULL,
    sms_method TEXT NOT NULL,
    sms_fallback_url TEXT,
    sms_fallback_method TEXT,
    status_callback TEXT,
    date_created TEXT NOT NULL,
    date_updated TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
    sid TEXT PRIMARY KEY,
    account_sid TEXT NOT NULL,
    messaging_service_sid TEXT,
    to_number TEXT NOT NULL,
    from_number TEXT NOT NULL,
    body TEXT NOT NULL,
    status TEXT NOT NULL,
    num_segments INTEGER NOT NULL,
    num_media INTEGER NOT NULL,
    direction TEXT NOT NULL,
    date_created TEXT NOT NULL,
    date_updated TEXT NOT NULL,
    date_sent TEXT,
    engine TEXT NOT NULL DEFAULT 'twilio',
    interactive_content TEXT,
    voice_metadata TEXT,
    telegram_metadata TEXT
);

CREATE INDEX IF NOT EXISTS idx_messages_engine ON messages(engine, to_number, from_number);

CREATE TABLE IF NOT EXISTS media (
    sid TEXT PRIMARY KEY,
    account_sid TEXT NOT NULL,
    parent_sid TEXT NOT NULL,
    content_type TEXT NOT NULL,
    data BLOB,
    date_created TEXT NOT NULL,
    date_updated TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS opt_out (
    account_sid TEXT NOT NULL,
    phone_number TEXT NOT NULL,
    opted_out INTEGER NOT NULL,
    PRIMARY KEY (account_sid, phone_number)
);

CREATE TABLE IF NOT EXISTS messaging_services (
    sid TEXT PRIMARY KEY,
    account_sid TEXT NOT NULL,
    friendly_name TEXT NOT NULL,
    inbound_request_url TEXT,
    inbound_method TEXT,
    fallback_url TEXT,
    fallback_method TEXT,
    status_callback TEXT,
    sticky_sender INTEGER NOT NULL,
    geomatch INTEGER NOT NULL,
    date_created TEXT NOT NULL,
    date_updated TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS service_phone_numbers (
    service_sid TEXT NOT NULL,
    phone_number_sid TEXT NOT NULL,
    phone_number TEXT NOT NULL,
    date_created TEXT NOT NULL,
    PRIMARY KEY (service_sid, phone_number_sid)
);
`

// Open opens or creates an SQLite database at dataSourceName and executes initial migrations.
func Open(dataSourceName string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(schema); err != nil {
		_ = conn.Close()
		return nil, err
	}
	_, _ = conn.Exec(`ALTER TABLE messages ADD COLUMN engine TEXT NOT NULL DEFAULT 'twilio'`)
	_, _ = conn.Exec(`ALTER TABLE messages ADD COLUMN interactive_content TEXT`)
	_, _ = conn.Exec(`ALTER TABLE messages ADD COLUMN voice_metadata TEXT`)
	_, _ = conn.Exec(`ALTER TABLE messages ADD COLUMN telegram_metadata TEXT`)
	_, _ = conn.Exec(`CREATE INDEX IF NOT EXISTS idx_messages_engine ON messages(engine, to_number, from_number)`)
	return &DB{db: conn}, nil
}

// Close terminates the database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// --- Messages ---

func scanMessageRow(scanner interface{ Scan(dest ...any) error }) (*domain.Message, error) {
	var (
		m                                                domain.Message
		status, direction                                string
		createdStr, updatedStr                           string
		dateSentStr, msgSvcSID                           sql.NullString
		engineStr, interactiveStr, voiceStr, telegramStr sql.NullString
	)
	if err := scanner.Scan(
		&m.SID, &m.AccountSID, &msgSvcSID, &m.To, &m.From, &m.Body,
		&status, &m.NumSegments, &m.NumMedia, &direction,
		&createdStr, &updatedStr, &dateSentStr,
		&engineStr, &interactiveStr, &voiceStr, &telegramStr,
	); err != nil {
		return nil, err
	}
	m.Status = domain.MessageStatus(status)
	m.Direction = domain.MessageDirection(direction)
	m.MessagingServiceSID = msgSvcSID.String
	m.DateCreated, _ = time.Parse(time.RFC3339, createdStr)
	m.DateUpdated, _ = time.Parse(time.RFC3339, updatedStr)
	if dateSentStr.Valid {
		t, _ := time.Parse(time.RFC3339, dateSentStr.String)
		m.DateSent = &t
	}
	if engineStr.Valid && engineStr.String != "" {
		m.Engine = domain.Engine(engineStr.String)
	} else {
		m.Engine = domain.EngineTwilio
	}
	if interactiveStr.Valid && interactiveStr.String != "" {
		var ic domain.InteractiveContent
		if err := json.Unmarshal([]byte(interactiveStr.String), &ic); err == nil {
			m.InteractiveContent = &ic
		}
	}
	if voiceStr.Valid && voiceStr.String != "" {
		var vm domain.VoiceMetadata
		if err := json.Unmarshal([]byte(voiceStr.String), &vm); err == nil {
			m.VoiceMetadata = &vm
		}
	}
	if telegramStr.Valid && telegramStr.String != "" {
		var tm domain.TelegramMetadata
		if err := json.Unmarshal([]byte(telegramStr.String), &tm); err == nil {
			m.TelegramMetadata = &tm
		}
	}
	m.PriceUnit = "USD"
	return &m, nil
}

// CreateMessage inserts a message into the database.
func (d *DB) CreateMessage(ctx context.Context, msg *domain.Message) error {
	query := `INSERT INTO messages (sid, account_sid, messaging_service_sid, to_number, from_number, body, status, num_segments, num_media, direction, date_created, date_updated, date_sent, engine, interactive_content, voice_metadata, telegram_metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	var dateSent *string
	if msg.DateSent != nil {
		s := msg.DateSent.Format(time.RFC3339)
		dateSent = &s
	}
	if msg.Engine == "" {
		msg.Engine = domain.EngineTwilio
	}

	var interactiveJSON, voiceJSON, telegramJSON *string
	if msg.InteractiveContent != nil {
		if b, err := json.Marshal(msg.InteractiveContent); err == nil {
			s := string(b)
			interactiveJSON = &s
		}
	}
	if msg.VoiceMetadata != nil {
		if b, err := json.Marshal(msg.VoiceMetadata); err == nil {
			s := string(b)
			voiceJSON = &s
		}
	}
	if msg.TelegramMetadata != nil {
		if b, err := json.Marshal(msg.TelegramMetadata); err == nil {
			s := string(b)
			telegramJSON = &s
		}
	}

	_, err := d.db.ExecContext(ctx, query,
		msg.SID, msg.AccountSID, msg.MessagingServiceSID,
		msg.To, msg.From, msg.Body, string(msg.Status),
		msg.NumSegments, msg.NumMedia, string(msg.Direction),
		msg.DateCreated.Format(time.RFC3339),
		msg.DateUpdated.Format(time.RFC3339),
		dateSent,
		string(msg.Engine),
		interactiveJSON,
		voiceJSON,
		telegramJSON,
	)
	return err
}

// GetMessageBySID retrieves a message by SID and account.
func (d *DB) GetMessageBySID(ctx context.Context, accountSID, sid string) (*domain.Message, error) {
	query := `SELECT sid, account_sid, messaging_service_sid, to_number, from_number, body, status, num_segments, num_media, direction, date_created, date_updated, date_sent, engine, interactive_content, voice_metadata, telegram_metadata
		FROM messages WHERE account_sid = ? AND sid = ?`

	row := d.db.QueryRowContext(ctx, query, accountSID, sid)
	m, err := scanMessageRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, err
	}
	return m, nil
}

// GetMessageByEngineID retrieves a message by engine and engine-specific ID.
func (d *DB) GetMessageByEngineID(ctx context.Context, engine domain.Engine, engineID string) (*domain.Message, error) {
	query := `SELECT sid, account_sid, messaging_service_sid, to_number, from_number, body, status, num_segments, num_media, direction, date_created, date_updated, date_sent, engine, interactive_content, voice_metadata, telegram_metadata
		FROM messages WHERE sid = ? AND (engine = ? OR (? = 'twilio' AND (engine = '' OR engine IS NULL)))`

	row := d.db.QueryRowContext(ctx, query, engineID, string(engine), string(engine))
	m, err := scanMessageRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, err
	}
	return m, nil
}

// ListMessages returns messages matching filter criteria.
func (d *DB) ListMessages(ctx context.Context, accountSID string, filter domain.MessageFilter) ([]*domain.Message, int, error) {
	query := `SELECT sid, account_sid, messaging_service_sid, to_number, from_number, body, status, num_segments, num_media, direction, date_created, date_updated, date_sent, engine, interactive_content, voice_metadata, telegram_metadata
		FROM messages WHERE (? = '' OR account_sid = ?) ORDER BY date_created DESC`

	rows, err := d.db.QueryContext(ctx, query, accountSID, accountSID)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	matched := make([]*domain.Message, 0)
	for rows.Next() {
		m, err := scanMessageRow(rows)
		if err != nil {
			continue
		}
		if filter.Engine != "" && m.Engine != filter.Engine {
			continue
		}
		if filter.To != "" && m.To != filter.To {
			continue
		}
		if filter.From != "" && m.From != filter.From {
			continue
		}
		if filter.DateSent != nil {
			if m.DateSent == nil || !m.DateSent.Equal(*filter.DateSent) {
				continue
			}
		}
		matched = append(matched, m)
	}

	total := len(matched)
	if filter.Offset >= total {
		return []*domain.Message{}, total, nil
	}
	matched = matched[filter.Offset:]
	if filter.Limit > 0 && len(matched) > filter.Limit {
		matched = matched[:filter.Limit]
	}
	return matched, total, nil
}

// UpdateMessage updates status and body of a message.
func (d *DB) UpdateMessage(ctx context.Context, msg *domain.Message) error {
	var interactiveJSON, voiceJSON, telegramJSON *string
	if msg.InteractiveContent != nil {
		if b, err := json.Marshal(msg.InteractiveContent); err == nil {
			s := string(b)
			interactiveJSON = &s
		}
	}
	if msg.VoiceMetadata != nil {
		if b, err := json.Marshal(msg.VoiceMetadata); err == nil {
			s := string(b)
			voiceJSON = &s
		}
	}
	if msg.TelegramMetadata != nil {
		if b, err := json.Marshal(msg.TelegramMetadata); err == nil {
			s := string(b)
			telegramJSON = &s
		}
	}
	if msg.Engine == "" {
		msg.Engine = domain.EngineTwilio
	}

	query := `UPDATE messages SET status = ?, body = ?, date_updated = ?, engine = ?, interactive_content = ?, voice_metadata = ?, telegram_metadata = ? WHERE account_sid = ? AND sid = ?`
	res, err := d.db.ExecContext(ctx, query, string(msg.Status), msg.Body, msg.DateUpdated.Format(time.RFC3339), string(msg.Engine), interactiveJSON, voiceJSON, telegramJSON, msg.AccountSID, msg.SID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// DeleteMessage deletes a message from the database.
func (d *DB) DeleteMessage(ctx context.Context, accountSID, sid string) error {
	res, err := d.db.ExecContext(ctx, `DELETE FROM messages WHERE account_sid = ? AND sid = ?`, accountSID, sid)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// --- Phone Numbers ---

func scanPhoneNumber(row *sql.Row) (*domain.PhoneNumber, error) {
	var (
		pn                                 domain.PhoneNumber
		createdStr, updatedStr             string
		fallbackURL, fallbackMethod, cbURL sql.NullString
	)
	err := row.Scan(
		&pn.SID, &pn.AccountSID, &pn.PhoneNumber, &pn.FriendlyName, &pn.Status,
		&pn.SmsURL, &pn.SmsMethod, &fallbackURL, &fallbackMethod, &cbURL,
		&createdStr, &updatedStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, err
	}
	pn.SmsFallbackURL = fallbackURL.String
	pn.SmsFallbackMethod = fallbackMethod.String
	pn.StatusCallback = cbURL.String
	pn.DateCreated, _ = time.Parse(time.RFC3339, createdStr)
	pn.DateUpdated, _ = time.Parse(time.RFC3339, updatedStr)
	pn.Capabilities = domain.Capabilities{Voice: true, SMS: true, MMS: true}
	return &pn, nil
}

// CreatePhoneNumber provisions a new phone number.
func (d *DB) CreatePhoneNumber(ctx context.Context, pn *domain.PhoneNumber) error {
	query := `INSERT INTO phone_numbers (sid, account_sid, phone_number, friendly_name, status, sms_url, sms_method, sms_fallback_url, sms_fallback_method, status_callback, date_created, date_updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := d.db.ExecContext(ctx, query,
		pn.SID, pn.AccountSID, pn.PhoneNumber, pn.FriendlyName, pn.Status,
		pn.SmsURL, pn.SmsMethod, pn.SmsFallbackURL, pn.SmsFallbackMethod, pn.StatusCallback,
		pn.DateCreated.Format(time.RFC3339), pn.DateUpdated.Format(time.RFC3339),
	)
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return repository.ErrConflict
	}
	return err
}

// GetPhoneNumberBySID retrieves a phone number by SID and account.
func (d *DB) GetPhoneNumberBySID(ctx context.Context, accountSID, sid string) (*domain.PhoneNumber, error) {
	query := `SELECT sid, account_sid, phone_number, friendly_name, status, sms_url, sms_method, sms_fallback_url, sms_fallback_method, status_callback, date_created, date_updated
		FROM phone_numbers WHERE account_sid = ? AND sid = ?`
	return scanPhoneNumber(d.db.QueryRowContext(ctx, query, accountSID, sid))
}

// GetPhoneNumberByNumber retrieves a phone number by account and number.
func (d *DB) GetPhoneNumberByNumber(ctx context.Context, accountSID, number string) (*domain.PhoneNumber, error) {
	query := `SELECT sid, account_sid, phone_number, friendly_name, status, sms_url, sms_method, sms_fallback_url, sms_fallback_method, status_callback, date_created, date_updated
		FROM phone_numbers WHERE account_sid = ? AND phone_number = ?`
	return scanPhoneNumber(d.db.QueryRowContext(ctx, query, accountSID, number))
}

// FindPhoneNumberByNumber retrieves a phone number by number string across accounts.
func (d *DB) FindPhoneNumberByNumber(ctx context.Context, number string) (*domain.PhoneNumber, error) {
	query := `SELECT sid, account_sid, phone_number, friendly_name, status, sms_url, sms_method, sms_fallback_url, sms_fallback_method, status_callback, date_created, date_updated
		FROM phone_numbers WHERE phone_number = ?`
	return scanPhoneNumber(d.db.QueryRowContext(ctx, query, number))
}

// ListPhoneNumbers returns phone numbers matching filter criteria.
func (d *DB) ListPhoneNumbers(ctx context.Context, accountSID string, filter domain.PhoneNumberFilter) ([]*domain.PhoneNumber, int, error) {
	query := `SELECT sid, account_sid, phone_number, friendly_name, status, sms_url, sms_method, sms_fallback_url, sms_fallback_method, status_callback, date_created, date_updated
		FROM phone_numbers WHERE account_sid = ? ORDER BY date_created DESC`

	rows, err := d.db.QueryContext(ctx, query, accountSID)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	matched := make([]*domain.PhoneNumber, 0)
	for rows.Next() {
		var (
			pn                                 domain.PhoneNumber
			createdStr, updatedStr             string
			fallbackURL, fallbackMethod, cbURL sql.NullString
		)
		if err := rows.Scan(
			&pn.SID, &pn.AccountSID, &pn.PhoneNumber, &pn.FriendlyName, &pn.Status,
			&pn.SmsURL, &pn.SmsMethod, &fallbackURL, &fallbackMethod, &cbURL,
			&createdStr, &updatedStr,
		); err != nil {
			continue
		}
		pn.SmsFallbackURL = fallbackURL.String
		pn.SmsFallbackMethod = fallbackMethod.String
		pn.StatusCallback = cbURL.String
		pn.DateCreated, _ = time.Parse(time.RFC3339, createdStr)
		pn.DateUpdated, _ = time.Parse(time.RFC3339, updatedStr)
		pn.Capabilities = domain.Capabilities{Voice: true, SMS: true, MMS: true}
		matched = append(matched, &pn)
	}

	total := len(matched)
	if filter.Offset >= total {
		return []*domain.PhoneNumber{}, total, nil
	}
	matched = matched[filter.Offset:]
	if filter.Limit > 0 && len(matched) > filter.Limit {
		matched = matched[:filter.Limit]
	}
	return matched, total, nil
}

// UpdatePhoneNumber updates an existing phone number.
func (d *DB) UpdatePhoneNumber(ctx context.Context, pn *domain.PhoneNumber) error {
	query := `UPDATE phone_numbers SET friendly_name = ?, sms_url = ?, sms_method = ?, status_callback = ?, date_updated = ?
		WHERE account_sid = ? AND sid = ?`
	res, err := d.db.ExecContext(ctx, query,
		pn.FriendlyName, pn.SmsURL, pn.SmsMethod, pn.StatusCallback,
		pn.DateUpdated.Format(time.RFC3339), pn.AccountSID, pn.SID,
	)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// DeletePhoneNumber deletes a phone number from the database.
func (d *DB) DeletePhoneNumber(ctx context.Context, accountSID, sid string) error {
	res, err := d.db.ExecContext(ctx, `DELETE FROM phone_numbers WHERE account_sid = ? AND sid = ?`, accountSID, sid)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// --- Media ---

// CreateMedia stores a media attachment record.
func (d *DB) CreateMedia(ctx context.Context, media *domain.Media) error {
	query := `INSERT INTO media (sid, account_sid, parent_sid, content_type, data, date_created, date_updated)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := d.db.ExecContext(ctx, query,
		media.SID, media.AccountSID, media.ParentSID, media.ContentType, media.Data,
		media.DateCreated.Format(time.RFC3339), media.DateUpdated.Format(time.RFC3339),
	)
	return err
}

// GetMediaBySID retrieves media by account, parent SID, and media SID.
func (d *DB) GetMediaBySID(ctx context.Context, accountSID, parentSID, sid string) (*domain.Media, error) {
	query := `SELECT sid, account_sid, parent_sid, content_type, data, date_created, date_updated
		FROM media WHERE account_sid = ? AND parent_sid = ? AND sid = ?`

	var (
		m                      domain.Media
		createdStr, updatedStr string
	)
	err := d.db.QueryRowContext(ctx, query, accountSID, parentSID, sid).Scan(
		&m.SID, &m.AccountSID, &m.ParentSID, &m.ContentType, &m.Data,
		&createdStr, &updatedStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, err
	}
	m.DateCreated, _ = time.Parse(time.RFC3339, createdStr)
	m.DateUpdated, _ = time.Parse(time.RFC3339, updatedStr)
	return &m, nil
}

// ListMedia returns media attachments for a parent message.
func (d *DB) ListMedia(ctx context.Context, accountSID, parentSID string) ([]*domain.Media, error) {
	query := `SELECT sid, account_sid, parent_sid, content_type, data, date_created, date_updated
		FROM media WHERE account_sid = ? AND parent_sid = ? ORDER BY date_created ASC`

	rows, err := d.db.QueryContext(ctx, query, accountSID, parentSID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var list []*domain.Media
	for rows.Next() {
		var (
			m                      domain.Media
			createdStr, updatedStr string
		)
		if err := rows.Scan(
			&m.SID, &m.AccountSID, &m.ParentSID, &m.ContentType, &m.Data,
			&createdStr, &updatedStr,
		); err != nil {
			continue
		}
		m.DateCreated, _ = time.Parse(time.RFC3339, createdStr)
		m.DateUpdated, _ = time.Parse(time.RFC3339, updatedStr)
		list = append(list, &m)
	}
	return list, nil
}

// DeleteMedia deletes a media attachment.
func (d *DB) DeleteMedia(ctx context.Context, accountSID, parentSID, sid string) error {
	res, err := d.db.ExecContext(ctx, `DELETE FROM media WHERE account_sid = ? AND parent_sid = ? AND sid = ?`, accountSID, parentSID, sid)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// --- OptOut ---

// SetOptOut records the opt-out preference for a phone number.
func (d *DB) SetOptOut(ctx context.Context, accountSID, phoneNumber string, optedOut bool) error {
	if !optedOut {
		_, err := d.db.ExecContext(ctx, `DELETE FROM opt_out WHERE account_sid = ? AND phone_number = ?`, accountSID, phoneNumber)
		return err
	}
	query := `INSERT INTO opt_out (account_sid, phone_number, opted_out) VALUES (?, ?, 1)
		ON CONFLICT(account_sid, phone_number) DO UPDATE SET opted_out = 1`
	_, err := d.db.ExecContext(ctx, query, accountSID, phoneNumber)
	return err
}

// IsOptedOut checks if a recipient number is opted out.
func (d *DB) IsOptedOut(ctx context.Context, accountSID, phoneNumber string) (bool, error) {
	var opted int
	err := d.db.QueryRowContext(ctx, `SELECT opted_out FROM opt_out WHERE account_sid = ? AND phone_number = ?`, accountSID, phoneNumber).Scan(&opted)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return opted == 1, nil
}

// --- Messaging Services ---

// CreateMessagingService inserts a messaging service.
func (d *DB) CreateMessagingService(ctx context.Context, svc *domain.MessagingService) error {
	query := `INSERT INTO messaging_services (sid, account_sid, friendly_name, inbound_request_url, inbound_method, fallback_url, fallback_method, status_callback, sticky_sender, geomatch, date_created, date_updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	sticky := 0
	if svc.StickySender {
		sticky = 1
	}
	geomatch := 0
	if svc.Geomatch {
		geomatch = 1
	}

	_, err := d.db.ExecContext(ctx, query,
		svc.SID, svc.AccountSID, svc.FriendlyName,
		svc.InboundRequestURL, svc.InboundMethod,
		svc.FallbackURL, svc.FallbackMethod,
		svc.StatusCallback, sticky, geomatch,
		svc.DateCreated.Format(time.RFC3339),
		svc.DateUpdated.Format(time.RFC3339),
	)
	return err
}

// GetMessagingServiceBySID retrieves a messaging service by SID.
func (d *DB) GetMessagingServiceBySID(ctx context.Context, accountSID, sid string) (*domain.MessagingService, error) {
	query := `SELECT sid, account_sid, friendly_name, inbound_request_url, inbound_method, fallback_url, fallback_method, status_callback, sticky_sender, geomatch, date_created, date_updated
		FROM messaging_services WHERE account_sid = ? AND sid = ?`

	var (
		s                                                 domain.MessagingService
		sticky, geomatch                                  int
		createdStr, updatedStr                            string
		inboundURL, inboundMethod, fbURL, fbMethod, cbURL sql.NullString
	)
	err := d.db.QueryRowContext(ctx, query, accountSID, sid).Scan(
		&s.SID, &s.AccountSID, &s.FriendlyName,
		&inboundURL, &inboundMethod, &fbURL, &fbMethod, &cbURL,
		&sticky, &geomatch, &createdStr, &updatedStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, err
	}
	s.InboundRequestURL = inboundURL.String
	s.InboundMethod = inboundMethod.String
	s.FallbackURL = fbURL.String
	s.FallbackMethod = fbMethod.String
	s.StatusCallback = cbURL.String
	s.StickySender = sticky == 1
	s.Geomatch = geomatch == 1
	s.DateCreated, _ = time.Parse(time.RFC3339, createdStr)
	s.DateUpdated, _ = time.Parse(time.RFC3339, updatedStr)
	return &s, nil
}

// ListMessagingServices lists messaging services for an account.
func (d *DB) ListMessagingServices(ctx context.Context, accountSID string, limit, offset int) ([]*domain.MessagingService, int, error) {
	query := `SELECT sid, account_sid, friendly_name, inbound_request_url, inbound_method, fallback_url, fallback_method, status_callback, sticky_sender, geomatch, date_created, date_updated
		FROM messaging_services WHERE account_sid = ? ORDER BY date_created DESC`

	rows, err := d.db.QueryContext(ctx, query, accountSID)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	matched := make([]*domain.MessagingService, 0)
	for rows.Next() {
		var (
			s                                                 domain.MessagingService
			sticky, geomatch                                  int
			createdStr, updatedStr                            string
			inboundURL, inboundMethod, fbURL, fbMethod, cbURL sql.NullString
		)
		if err := rows.Scan(
			&s.SID, &s.AccountSID, &s.FriendlyName,
			&inboundURL, &inboundMethod, &fbURL, &fbMethod, &cbURL,
			&sticky, &geomatch, &createdStr, &updatedStr,
		); err != nil {
			continue
		}
		s.InboundRequestURL = inboundURL.String
		s.InboundMethod = inboundMethod.String
		s.FallbackURL = fbURL.String
		s.FallbackMethod = fbMethod.String
		s.StatusCallback = cbURL.String
		s.StickySender = sticky == 1
		s.Geomatch = geomatch == 1
		s.DateCreated, _ = time.Parse(time.RFC3339, createdStr)
		s.DateUpdated, _ = time.Parse(time.RFC3339, updatedStr)
		matched = append(matched, &s)
	}

	total := len(matched)
	if offset >= total {
		return []*domain.MessagingService{}, total, nil
	}
	matched = matched[offset:]
	if limit > 0 && len(matched) > limit {
		matched = matched[:limit]
	}
	return matched, total, nil
}

// UpdateMessagingService updates an existing messaging service.
func (d *DB) UpdateMessagingService(ctx context.Context, svc *domain.MessagingService) error {
	query := `UPDATE messaging_services SET friendly_name = ?, sticky_sender = ?, geomatch = ?, date_updated = ?
		WHERE account_sid = ? AND sid = ?`
	sticky := 0
	if svc.StickySender {
		sticky = 1
	}
	geomatch := 0
	if svc.Geomatch {
		geomatch = 1
	}
	res, err := d.db.ExecContext(ctx, query, svc.FriendlyName, sticky, geomatch, svc.DateUpdated.Format(time.RFC3339), svc.AccountSID, svc.SID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// DeleteMessagingService deletes a messaging service.
func (d *DB) DeleteMessagingService(ctx context.Context, accountSID, sid string) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, _ = tx.ExecContext(ctx, `DELETE FROM service_phone_numbers WHERE service_sid = ?`, sid)
	res, err := tx.ExecContext(ctx, `DELETE FROM messaging_services WHERE account_sid = ? AND sid = ?`, accountSID, sid)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return repository.ErrNotFound
	}
	return tx.Commit()
}

// AddServicePhoneNumber binds a phone number to a service pool.
func (d *DB) AddServicePhoneNumber(ctx context.Context, spn *domain.ServicePhoneNumber) error {
	query := `INSERT INTO service_phone_numbers (service_sid, phone_number_sid, phone_number, date_created)
		VALUES (?, ?, ?, ?)`
	_, err := d.db.ExecContext(ctx, query, spn.ServiceSID, spn.PhoneNumberSID, spn.PhoneNumber, spn.DateCreated.Format(time.RFC3339))
	if err != nil && strings.Contains(err.Error(), "PRIMARY KEY") {
		return repository.ErrConflict
	}
	return err
}

// ListServicePhoneNumbers returns numbers bound to a service pool.
func (d *DB) ListServicePhoneNumbers(ctx context.Context, serviceSID string) ([]*domain.ServicePhoneNumber, error) {
	query := `SELECT service_sid, phone_number_sid, phone_number, date_created
		FROM service_phone_numbers WHERE service_sid = ? ORDER BY date_created ASC`

	rows, err := d.db.QueryContext(ctx, query, serviceSID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var list []*domain.ServicePhoneNumber
	for rows.Next() {
		var (
			spn        domain.ServicePhoneNumber
			createdStr string
		)
		if err := rows.Scan(&spn.ServiceSID, &spn.PhoneNumberSID, &spn.PhoneNumber, &createdStr); err != nil {
			continue
		}
		spn.DateCreated, _ = time.Parse(time.RFC3339, createdStr)
		list = append(list, &spn)
	}
	return list, nil
}

// RemoveServicePhoneNumber unbinds a phone number from a service pool.
func (d *DB) RemoveServicePhoneNumber(ctx context.Context, serviceSID, phoneNumberSID string) error {
	res, err := d.db.ExecContext(ctx, `DELETE FROM service_phone_numbers WHERE service_sid = ? AND phone_number_sid = ?`, serviceSID, phoneNumberSID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return repository.ErrNotFound
	}
	return nil
}
