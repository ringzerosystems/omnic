// Command omnic-handset is a CLI and Bubble Tea TUI client that emulates a mobile phone handset.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/ffmpeg"
)

var version = "dev-dirty"

func main() {
	os.Exit(run())
}

func run() int {
	loadEnvFile(".env")
	versionFlag := flag.Bool("version", false, "print version and exit")
	server := flag.String("server", getEnvDefault("OMPHONE_SERVER", "http://localhost:8080"), "omnic server base URL")
	from := flag.String("from", getEnvDefault("OMPHONE_FROM", "+14155551111"), "sender phone number")
	to := flag.String("to", getEnvDefault("OMPHONE_TO", "+14155550199"), "recipient phone number")
	body := flag.String("body", "", "message body for one-shot CLI send")
	engine := flag.String("engine", getEnvDefault("OMPHONE_ENGINE", "all"), "handset protocol engine: twilio, whatsapp, telegram, or all")
	audioIn := flag.String("audio-in", getEnvDefault("OMPHONE_AUDIO_IN", "default"), "audio input device ID or name")
	audioOut := flag.String("audio-out", getEnvDefault("OMPHONE_AUDIO_OUT", "default"), "audio output device ID or name")
	tgChatID := flag.Int64("telegram-chat-id", getEnvInt64Default("OMPHONE_TELEGRAM_CHAT_ID", 14155551111), "default Telegram chat ID representing handset user")
	tgUsername := flag.String("telegram-username", getEnvDefault("OMPHONE_TELEGRAM_USERNAME", "handset_user"), "Telegram username of handset participant")
	listAudio := flag.Bool("list-audio-devices", false, "list discovered audio input/output devices and exit")
	headless := flag.Bool("headless", false, "run in headless streaming mode (no TUI)")
	flag.Parse()

	if *versionFlag {
		fmt.Println(version)
		return 0
	}

	if *listAudio {
		return printAudioDevices()
	}

	cfg := phoneConfig{
		Server:           *server,
		From:             *from,
		To:               *to,
		Engine:           *engine,
		AudioIn:          *audioIn,
		AudioOut:         *audioOut,
		TelegramUsername: *tgUsername,
		TelegramChatID:   *tgChatID,
	}

	if *body != "" {
		return runOneShotSend(cfg, *body)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if *headless {
		return runHeadlessStream(cfg, ctx)
	}

	m := newPhoneModel(cfg)
	m.ctx = ctx
	p := tea.NewProgram(m, tea.WithContext(ctx))
	if _, err := p.Run(); err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func getEnvDefault(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	if after, ok := strings.CutPrefix(key, "OMPHONE_"); ok {
		if val := os.Getenv("TWPHONE_" + after); val != "" {
			return val
		}
	}
	return def
}

func getEnvInt64Default(key string, def int64) int64 {
	val := os.Getenv(key)
	if val == "" && strings.HasPrefix(key, "OMPHONE_") {
		val = os.Getenv("TWPHONE_" + strings.TrimPrefix(key, "OMPHONE_"))
	}
	if val != "" {
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func loadEnvFile(path string) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}

func printAudioDevices() int {
	dm := ffmpeg.NewDeviceManager()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	inputs, _ := dm.ListInputDevices(ctx)
	outputs, _ := dm.ListOutputDevices(ctx)

	fmt.Println("Audio Input Devices:")
	if len(inputs) == 0 {
		fmt.Println("  (none found)")
	}
	for _, d := range inputs {
		def := ""
		if d.IsDefault {
			def = " (default)"
		}
		fmt.Printf("  - [%s] %s%s\n", d.ID, d.Name, def)
	}

	fmt.Println("\nAudio Output Devices:")
	if len(outputs) == 0 {
		fmt.Println("  (none found)")
	}
	for _, d := range outputs {
		def := ""
		if d.IsDefault {
			def = " (default)"
		}
		fmt.Printf("  - [%s] %s%s\n", d.ID, d.Name, def)
	}
	return 0
}

func runOneShotSend(cfg phoneConfig, body string) int {
	eng := strings.ToLower(cfg.Engine)
	if eng == "twilio" || eng == "all" {
		reply, err := sendInboundSMS(cfg.Server, cfg.From, cfg.To, body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Twilio send error: %v\n", err)
			return 1
		}
		fmt.Printf("Sent SMS to %s: %s\n", cfg.To, body)
		if reply != "" {
			fmt.Printf("Received Twilio Reply: %s\n", reply)
		}
	}
	if eng == "whatsapp" || eng == "all" {
		reply, err := sendInboundWhatsApp(cfg.Server, whatsAppInboundPayload{
			From: cfg.From,
			To:   cfg.To,
			Type: "text",
			Body: body,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "WhatsApp send error: %v\n", err)
			return 1
		}
		fmt.Printf("Sent WhatsApp message to %s: %s\n", cfg.To, body)
		if reply != "" {
			fmt.Printf("Received WhatsApp Reply: %s\n", reply)
		}
	}
	if eng == "telegram" || eng == "all" {
		reply, err := sendInboundTelegram(cfg.Server, telegramInboundPayload{
			FromID:    cfg.TelegramChatID,
			ChatID:    cfg.TelegramChatID,
			Username:  cfg.TelegramUsername,
			FirstName: "Handset User",
			Text:      body,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Telegram send error: %v\n", err)
			return 1
		}
		fmt.Printf("Sent Telegram message to %d: %s\n", cfg.TelegramChatID, body)
		if reply != "" {
			fmt.Printf("Received Telegram Reply: %s\n", reply)
		}
	}
	return 0
}

type telegramInboundPayload struct {
	Username        string  `json:"username,omitempty"`
	FirstName       string  `json:"first_name,omitempty"`
	Text            string  `json:"text,omitempty"`
	Type            string  `json:"type,omitempty"`
	MediaID         string  `json:"media_id,omitempty"`
	CallbackQueryID string  `json:"callback_query_id,omitempty"`
	CallbackData    string  `json:"callback_data,omitempty"`
	FromID          int64   `json:"from_id,omitempty"`
	ChatID          int64   `json:"chat_id,omitempty"`
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
	MessageID       int     `json:"message_id,omitempty"`
	Voice           bool    `json:"voice,omitempty"`
}

func sendInboundTelegram(server string, req telegramInboundPayload) (string, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	endpoint := strings.TrimRight(server, "/") + "/emu/inbound/telegram"
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(endpoint, "application/json", bytes.NewBuffer(payload))
	if err != nil || resp == nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return "", parseHTTPError(resp)
	}

	return parseReplyBody(resp.Body)
}

type whatsAppInboundPayload struct {
	From            string  `json:"from"`
	To              string  `json:"to"`
	Type            string  `json:"type"`
	Body            string  `json:"body"`
	MediaID         string  `json:"media_id,omitempty"`
	MediaURL        string  `json:"media_url,omitempty"`
	InteractiveType string  `json:"interactive_type,omitempty"`
	SelectedID      string  `json:"selected_id,omitempty"`
	SelectedTitle   string  `json:"selected_title,omitempty"`
	SelectedDesc    string  `json:"selected_description,omitempty"`
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
	Voice           bool    `json:"voice,omitempty"`
}

func sendInboundSMS(server, from, to, body string) (string, error) {
	payload, _ := json.Marshal(map[string]any{
		"from": from,
		"to":   to,
		"body": body,
	})

	endpoint := strings.TrimRight(server, "/") + "/emu/inbound/sms"
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(endpoint, "application/json", bytes.NewBuffer(payload))
	if err != nil || resp == nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return "", parseHTTPError(resp)
	}

	return parseReplyBody(resp.Body)
}

func sendInboundWhatsApp(server string, req whatsAppInboundPayload) (string, error) {
	payload, _ := json.Marshal(req)
	endpoint := strings.TrimRight(server, "/") + "/emu/inbound/whatsapp"
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(endpoint, "application/json", bytes.NewBuffer(payload))
	if err != nil || resp == nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return "", parseHTTPError(resp)
	}

	return parseReplyBody(resp.Body)
}

func parseHTTPError(resp *http.Response) error {
	b, _ := io.ReadAll(resp.Body)
	var errPayload struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	}
	if err := json.Unmarshal(b, &errPayload); err == nil && errPayload.Message != "" {
		if errPayload.Code != 0 {
			return fmt.Errorf("server error (%d, code %d): %s", resp.StatusCode, errPayload.Code, errPayload.Message)
		}
		return fmt.Errorf("server error (%d): %s", resp.StatusCode, errPayload.Message)
	}
	return fmt.Errorf("server error (%d): %s", resp.StatusCode, string(b))
}

func parseReplyBody(r io.Reader) (string, error) {
	var result map[string]any
	if err := json.NewDecoder(r).Decode(&result); err != nil {
		return "", nil
	}
	if reply, ok := result["reply"].(map[string]any); ok {
		if replyBody, ok := reply["body"].(string); ok {
			return replyBody, nil
		}
	}
	return "", nil
}

func sendTargetMessageCmd(cfg phoneConfig, body, target string) tea.Cmd {
	return func() tea.Msg {
		switch target {
		case targetTwilio:
			reply, err := sendInboundSMS(cfg.Server, cfg.From, cfg.To, body)
			return sendResultMsg{reply: reply, err: err}
		case targetWhatsApp:
			reply, err := sendInboundWhatsApp(cfg.Server, whatsAppInboundPayload{
				From: cfg.From,
				To:   cfg.To,
				Type: "text",
				Body: body,
			})
			return sendResultMsg{reply: reply, err: err}
		case targetTelegram:
			reply, err := sendInboundTelegram(cfg.Server, telegramInboundPayload{
				FromID:    cfg.TelegramChatID,
				ChatID:    cfg.TelegramChatID,
				Username:  cfg.TelegramUsername,
				FirstName: "Handset User",
				Text:      body,
			})
			return sendResultMsg{reply: reply, err: err}
		case targetAll:
			_, errSMS := sendInboundSMS(cfg.Server, cfg.From, cfg.To, body)
			replyWA, errWA := sendInboundWhatsApp(cfg.Server, whatsAppInboundPayload{
				From: cfg.From,
				To:   cfg.To,
				Type: "text",
				Body: body,
			})
			replyTG, errTG := sendInboundTelegram(cfg.Server, telegramInboundPayload{
				FromID:    cfg.TelegramChatID,
				ChatID:    cfg.TelegramChatID,
				Username:  cfg.TelegramUsername,
				FirstName: "Handset User",
				Text:      body,
			})
			var err error
			for _, e := range []error{errSMS, errWA, errTG} {
				if e != nil {
					err = errors.Join(err, e)
				}
			}
			reply := replyWA
			if reply == "" {
				reply = replyTG
			}
			return sendResultMsg{reply: reply, err: err}
		default:
			reply, err := sendInboundWhatsApp(cfg.Server, whatsAppInboundPayload{
				From: cfg.From,
				To:   cfg.To,
				Type: "text",
				Body: body,
			})
			return sendResultMsg{reply: reply, err: err}
		}
	}
}

func sendTelegramCallbackCmd(server string, chatID int64, username, cbData string) tea.Cmd {
	return func() tea.Msg {
		reply, err := sendInboundTelegram(server, telegramInboundPayload{
			FromID:          chatID,
			ChatID:          chatID,
			Username:        username,
			FirstName:       "Handset User",
			CallbackData:    cbData,
			CallbackQueryID: fmt.Sprintf("cbq_%d", time.Now().UnixNano()),
		})
		return sendResultMsg{reply: reply, err: err}
	}
}

func sendButtonReplyCmd(server, from, to, btnID, btnTitle string) tea.Cmd {
	return func() tea.Msg {
		reply, err := sendInboundWhatsApp(server, whatsAppInboundPayload{
			From:            from,
			To:              to,
			Type:            "interactive",
			InteractiveType: "button_reply",
			SelectedID:      btnID,
			SelectedTitle:   btnTitle,
		})
		return sendResultMsg{reply: reply, err: err}
	}
}

func sendListReplyCmd(server, from, to string, item menuItemOption) tea.Cmd {
	return func() tea.Msg {
		reply, err := sendInboundWhatsApp(server, whatsAppInboundPayload{
			From:            from,
			To:              to,
			Type:            "interactive",
			InteractiveType: "list_reply",
			SelectedID:      item.ID,
			SelectedTitle:   item.Title,
			SelectedDesc:    item.Description,
		})
		return sendResultMsg{reply: reply, err: err}
	}
}

func sendVoiceNoteCmd(cfg phoneConfig, filePath string, dur time.Duration, target string) tea.Cmd {
	return func() tea.Msg {
		defer func() { _ = os.Remove(filePath) }()
		if target == targetTelegram {
			reply, err := sendTelegramVoiceNote(cfg.Server, cfg.TelegramChatID, cfg.TelegramUsername, filePath, dur)
			return sendResultMsg{reply: reply, err: err}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		mediaID, _ := uploadAudioMedia(ctx, cfg.Server, "100000000000001", filePath)
		reply, err := sendInboundWhatsApp(cfg.Server, whatsAppInboundPayload{
			From:            cfg.From,
			To:              cfg.To,
			Type:            "audio",
			Body:            "[Voice Note]",
			MediaID:         mediaID,
			MediaURL:        filePath,
			DurationSeconds: dur.Seconds(),
			Voice:           true,
		})
		return sendResultMsg{reply: reply, err: err}
	}
}

func sendTelegramVoiceNote(server string, chatID int64, username, filePath string, dur time.Duration) (string, error) {
	//nolint:gosec // filePath is created internally by recorder
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	part, err := w.CreateFormFile("voice", filepath.Base(filePath))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", err
	}
	_ = w.WriteField("chat_id", strconv.FormatInt(chatID, 10))
	_ = w.WriteField("from_id", strconv.FormatInt(chatID, 10))
	_ = w.WriteField("username", username)
	_ = w.WriteField("first_name", "Handset User")
	_ = w.WriteField("voice", "true")
	_ = w.WriteField("duration_seconds", fmt.Sprintf("%.2f", dur.Seconds()))
	_ = w.Close()

	endpoint := strings.TrimRight(server, "/") + "/emu/inbound/telegram"
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(endpoint, w.FormDataContentType(), &b)
	if err != nil || resp == nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return "", parseHTTPError(resp)
	}
	return parseReplyBody(resp.Body)
}

func uploadAudioMedia(ctx context.Context, server, phoneID, filePath string) (string, error) {
	//nolint:gosec // filePath is created internally by recorder
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	part, err := w.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", err
	}
	_ = w.WriteField("messaging_product", "whatsapp")
	_ = w.WriteField("type", "audio/ogg")
	_ = w.Close()

	endpoint := fmt.Sprintf("%s/v21.0/%s/media", strings.TrimRight(server, "/"), phoneID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &b)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer wa_token_dev")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp == nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return "", parseHTTPError(resp)
	}

	var res struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	return res.ID, nil
}

func playAudioCmd(ctx context.Context, player *ffmpeg.Player, server, target string) tea.Cmd {
	return func() tea.Msg {
		playURL := target
		if strings.HasPrefix(target, "/") {
			playURL = fmt.Sprintf("%s%s", strings.TrimRight(server, "/"), target)
		} else if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
			playURL = fmt.Sprintf("%s/v21.0/media/%s/download", strings.TrimRight(server, "/"), target)
		}
		doneCh, err := player.Play(ctx, playURL)
		if err != nil {
			return playbackFinishedMsg{err: err}
		}
		err = <-doneCh
		return playbackFinishedMsg{err: err}
	}
}

func fetchHistoryCmd(ctx context.Context, server, from, to string) tea.Cmd {
	return func() tea.Msg {
		msgs, err := fetchHistory(ctx, server, from, to)
		if err != nil {
			return nil
		}
		return historyLoadedMsg{messages: msgs}
	}
}

func fetchHistory(ctx context.Context, server, from, to string) ([]chatMessage, error) {
	endpoint := fmt.Sprintf("%s/emu/messages", strings.TrimRight(server, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp == nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var data struct {
		Messages []struct {
			InteractiveContent *domain.InteractiveContent `json:"interactive_content"`
			VoiceMetadata      *domain.VoiceMetadata      `json:"voice_metadata"`
			TelegramMetadata   *domain.TelegramMetadata   `json:"telegram_metadata"`
			DateCreated        string                     `json:"date_created"`
			SID                string                     `json:"sid"`
			From               string                     `json:"from"`
			To                 string                     `json:"to"`
			Body               string                     `json:"body"`
			Status             string                     `json:"status"`
			Direction          string                     `json:"direction"`
			Engine             string                     `json:"engine"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var res []chatMessage
	for _, m := range data.Messages {
		if (m.From == from && m.To == to) || (m.From == to && m.To == from) || m.Engine == string(domain.EngineTelegram) {
			t, _ := time.Parse(time.RFC1123Z, m.DateCreated)
			eng := domain.EngineTwilio
			if m.Engine == string(domain.EngineWhatsApp) {
				eng = domain.EngineWhatsApp
			} else if m.Engine == string(domain.EngineTelegram) {
				eng = domain.EngineTelegram
			}
			res = append(res, chatMessage{
				Timestamp:          t,
				SID:                m.SID,
				From:               m.From,
				To:                 m.To,
				Body:               m.Body,
				Status:             m.Status,
				Direction:          m.Direction,
				Engine:             eng,
				InteractiveContent: m.InteractiveContent,
				VoiceMetadata:      m.VoiceMetadata,
				TelegramMetadata:   m.TelegramMetadata,
			})
		}
	}
	return res, nil
}

func startSSEListener(ctx context.Context, server, from, engine string, out chan<- sseEventMsg) {
	go func() {
		endpoint := fmt.Sprintf("%s/emu/events?number=%s", strings.TrimRight(server, "/"), url.QueryEscape(from))
		if engine != "" && engine != "all" {
			endpoint += "&engine=" + url.QueryEscape(engine)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return
		}
		client := &http.Client{Timeout: 0}
		resp, err := client.Do(req)
		if err != nil || resp == nil {
			return
		}
		defer func() { _ = resp.Body.Close() }()

		scanner := bufio.NewScanner(resp.Body)
		var currentType, currentID string
		for scanner.Scan() {
			line := scanner.Text()
			if after, ok := strings.CutPrefix(line, "event:"); ok {
				currentType = strings.TrimSpace(after)
			} else if after, ok := strings.CutPrefix(line, "id:"); ok {
				currentID = strings.TrimSpace(after)
			} else if after, ok := strings.CutPrefix(line, "data:"); ok {
				parseAndSendSSEEvent(currentType, currentID, strings.TrimSpace(after), out)
			}
		}
	}()
}

func parseAndSendSSEEvent(currentType, currentID, dataStr string, out chan<- sseEventMsg) {
	var payload struct {
		InteractiveContent *domain.InteractiveContent `json:"interactive_content"`
		VoiceMetadata      *domain.VoiceMetadata      `json:"voice_metadata"`
		TelegramMetadata   *domain.TelegramMetadata   `json:"telegram_metadata"`
		DateCreated        string                     `json:"date_created"`
		SID                string                     `json:"sid"`
		From               string                     `json:"from"`
		To                 string                     `json:"to"`
		Body               string                     `json:"body"`
		Status             string                     `json:"status"`
		Direction          string                     `json:"direction"`
		Engine             string                     `json:"engine"`
	}
	if err := json.Unmarshal([]byte(dataStr), &payload); err == nil {
		t, _ := time.Parse(time.RFC1123Z, payload.DateCreated)
		if payload.SID == "" {
			payload.SID = currentID
		}
		eng := domain.EngineTwilio
		if payload.Engine == string(domain.EngineWhatsApp) {
			eng = domain.EngineWhatsApp
		} else if payload.Engine == string(domain.EngineTelegram) {
			eng = domain.EngineTelegram
		}
		out <- sseEventMsg{
			EventType: currentType,
			Message: chatMessage{
				Timestamp:          t,
				SID:                payload.SID,
				From:               payload.From,
				To:                 payload.To,
				Body:               payload.Body,
				Status:             payload.Status,
				Direction:          payload.Direction,
				Engine:             eng,
				InteractiveContent: payload.InteractiveContent,
				VoiceMetadata:      payload.VoiceMetadata,
				TelegramMetadata:   payload.TelegramMetadata,
			},
		}
	}
}

func waitForSSE(ch <-chan sseEventMsg) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			return nil
		}
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return ev
	}
}

func runHeadlessStream(cfg phoneConfig, ctx context.Context) int {
	ch := make(chan sseEventMsg, 32)
	startSSEListener(ctx, cfg.Server, cfg.From, cfg.Engine, ch)
	fmt.Printf("[HEADLESS] Mobile handset %s listening on %s (engine: %s)\n", cfg.From, cfg.Server, cfg.Engine)
	for {
		select {
		case <-ctx.Done():
			return 0
		case ev, ok := <-ch:
			if !ok {
				return 0
			}
			fmt.Printf("[%s] from=%s to=%s engine=%s body=%s\n",
				ev.EventType, ev.Message.From, ev.Message.To, ev.Message.Engine, ev.Message.Body)
		}
	}
}
