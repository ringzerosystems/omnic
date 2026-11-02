package main

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/ffmpeg"
)

const (
	targetTwilio   = "twilio"
	targetWhatsApp = "whatsapp"
	targetTelegram = "telegram"
	targetAll      = "all"
)

type chatMessage struct {
	Timestamp          time.Time
	InteractiveContent *domain.InteractiveContent
	VoiceMetadata      *domain.VoiceMetadata
	TelegramMetadata   *domain.TelegramMetadata
	SID                string
	From               string
	To                 string
	Body               string
	Status             string
	Direction          string
	Engine             domain.Engine
	MediaURLs          []string
}

type sseEventMsg struct {
	EventType string
	Message   chatMessage
}

type historyLoadedMsg struct {
	messages []chatMessage
}

type sendResultMsg struct {
	err   error
	reply string
}

type recordTickMsg struct {
	t time.Time
}

type recordFinishedMsg struct {
	err      error
	filePath string
	duration time.Duration
}

type playbackFinishedMsg struct {
	err error
}

type menuItemOption struct {
	SectionTitle string
	ID           string
	Title        string
	Description  string
}

type phoneConfig struct {
	Server           string
	From             string
	To               string
	Engine           string
	AudioIn          string
	AudioOut         string
	TelegramUsername string
	TelegramChatID   int64
}

type model struct {
	ctx              context.Context
	sseChan          chan sseEventMsg
	player           *ffmpeg.Player
	recorder         *ffmpeg.Recorder
	cfg              phoneConfig
	server           string
	from             string
	to               string
	status           string
	input            string
	configuredEngine string
	target           string
	audioIn          string
	audioOut         string
	recordFile       string
	playingSID       string
	messages         []chatMessage
	menuItems        []menuItemOption
	width            int
	height           int
	scrollOffset     int
	menuCursor       int
	recordSeconds    int
	recordVU         int
	menuOpen         bool
	isPlaying        bool
	isRecording      bool
	showCockpit      bool
}

func newModel(server, from, to string) model {
	return newPhoneModel(phoneConfig{
		Server:           server,
		From:             from,
		To:               to,
		Engine:           "all",
		AudioIn:          "default",
		AudioOut:         "default",
		TelegramChatID:   14155551111,
		TelegramUsername: "handset_user",
	})
}

func newPhoneModel(cfg phoneConfig) model {
	if cfg.TelegramChatID == 0 {
		cfg.TelegramChatID = 14155551111
	}
	if cfg.TelegramUsername == "" {
		cfg.TelegramUsername = "handset_user"
	}
	eng := strings.ToLower(cfg.Engine)
	if eng == "" {
		eng = "all"
	}

	target := targetWhatsApp
	switch eng {
	case "twilio":
		target = targetTwilio
	case "telegram":
		target = targetTelegram
	}

	return model{
		ctx:              context.Background(),
		cfg:              cfg,
		server:           cfg.Server,
		from:             cfg.From,
		to:               cfg.To,
		status:           "Ready",
		configuredEngine: eng,
		target:           target,
		audioIn:          cfg.AudioIn,
		audioOut:         cfg.AudioOut,
		messages:         make([]chatMessage, 0),
		sseChan:          make(chan sseEventMsg, 32),
		player:           ffmpeg.NewPlayer(),
		recorder:         ffmpeg.NewRecorder(ffmpeg.RecordConfig{DeviceID: cfg.AudioIn}),
		width:            80,
		height:           24,
	}
}

func (m *model) addOrUpdateMessage(msg chatMessage) {
	for i := range m.messages {
		if msg.SID != "" && m.messages[i].SID == msg.SID {
			m.messages[i].Status = msg.Status
			if msg.Body != "" {
				m.messages[i].Body = msg.Body
			}
			if msg.InteractiveContent != nil {
				m.messages[i].InteractiveContent = msg.InteractiveContent
			}
			if msg.VoiceMetadata != nil {
				m.messages[i].VoiceMetadata = msg.VoiceMetadata
			}
			if msg.TelegramMetadata != nil {
				m.messages[i].TelegramMetadata = msg.TelegramMetadata
			}
			return
		}
	}
	m.messages = append(m.messages, msg)
}

func (m model) Init() tea.Cmd {
	if m.sseChan != nil {
		startSSEListener(m.ctx, m.server, m.from, m.configuredEngine, m.sseChan)
	}
	return tea.Batch(
		fetchHistoryCmd(m.ctx, m.server, m.from, m.to),
		waitForSSE(m.sseChan),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)
	case sseEventMsg:
		m.addOrUpdateMessage(msg.Message)
		if msg.EventType == "message.outbound" || msg.Message.Status == "delivered" {
			m.status = "Ready"
		}
		m.scrollOffset = 0
		return m, waitForSSE(m.sseChan)
	case historyLoadedMsg:
		for _, hm := range msg.messages {
			m.addOrUpdateMessage(hm)
		}
		return m, nil
	case sendResultMsg:
		return m.handleSendResult(msg)
	case recordTickMsg:
		if m.isRecording {
			m.recordSeconds++
			m.recordVU = (m.recordVU + 1) % 10
			return m, tickRecorder()
		}
		return m, nil
	case recordFinishedMsg:
		return m.handleRecordFinished(msg)
	case playbackFinishedMsg:
		m.isPlaying = false
		m.playingSID = ""
		m.status = "Playback finished"
		return m, nil
	}
	return m, nil
}

func (m model) handleKeyPress(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.menuOpen {
		return m.handleMenuKeyPress(key)
	}
	if m.isRecording {
		return m.handleRecordingKeyPress(key)
	}

	if key.Mod == tea.ModCtrl && (key.Code == 'k' || key.Code == 'K') {
		m.messages = make([]chatMessage, 0)
		m.scrollOffset = 0
		return m, nil
	}

	if (key.Code == tea.KeyTab) || (key.Mod == tea.ModCtrl && (key.Code == 's' || key.Code == 'S')) {
		m.cycleTarget()
		return m, nil
	}

	switch key.Code {
	case tea.KeyUp:
		m.scrollOffset++
		return m, nil
	case tea.KeyDown:
		if m.scrollOffset > 0 {
			m.scrollOffset--
		}
		return m, nil
	case tea.KeyPgUp:
		m.scrollOffset += 5
		return m, nil
	case tea.KeyPgDown:
		if m.scrollOffset > 5 {
			m.scrollOffset -= 5
		} else {
			m.scrollOffset = 0
		}
		return m, nil
	case 'c':
		if key.Mod == tea.ModCtrl {
			return m, tea.Quit
		}
	case tea.KeyEscape:
		return m, tea.Quit
	case tea.KeyBackspace:
		if len(m.input) > 0 {
			r := []rune(m.input)
			m.input = string(r[:len(r)-1])
		}
		return m, nil
	case tea.KeyEnter:
		return m.handleEnterKey()
	}

	if m.input == "" {
		if handled, nextM, cmd := m.handleSpecialShortcuts(key); handled {
			return nextM, cmd
		}
	}

	if key.Text != "" {
		m.input += key.Text
	} else if key.Code >= 32 && key.Code <= 126 {
		m.input += string(rune(key.Code))
	}
	return m, nil
}

func (m *model) cycleTarget() {
	if m.configuredEngine == "twilio" {
		m.target = targetTwilio
		return
	}
	if m.configuredEngine == "whatsapp" {
		m.target = targetWhatsApp
		return
	}
	if m.configuredEngine == "telegram" {
		m.target = targetTelegram
		return
	}
	switch m.target {
	case targetWhatsApp:
		m.target = targetTwilio
	case targetTwilio:
		m.target = targetTelegram
	case targetTelegram:
		m.target = targetAll
	case targetAll:
		m.target = targetWhatsApp
	default:
		m.target = targetWhatsApp
	}
}

func (m model) handleEnterKey() (tea.Model, tea.Cmd) {
	if strings.TrimSpace(m.input) == "" {
		return m, nil
	}
	text := m.input
	m.input = ""
	m.status = "AI Agent is thinking..."
	m.scrollOffset = 0

	eng := domain.EngineWhatsApp
	switch m.target {
	case targetTwilio:
		eng = domain.EngineTwilio
	case targetTelegram:
		eng = domain.EngineTelegram
	}

	m.addOrUpdateMessage(chatMessage{
		Timestamp: time.Now(),
		From:      m.from,
		To:        m.to,
		Body:      text,
		Status:    "sending",
		Direction: "inbound",
		Engine:    eng,
	})

	return m, sendTargetMessageCmd(m.cfg, text, m.target)
}

func (m model) handleSpecialShortcuts(key tea.KeyPressMsg) (bool, tea.Model, tea.Cmd) {
	if key.Mod == tea.ModCtrl && (key.Code == 'r' || key.Code == 'R') {
		newM, cmd := m.startRecording()
		return true, newM, cmd
	}
	switch key.Code {
	case '1', '2', '3':
		idx := int(key.Code - '1')
		if cmd := m.triggerButtonReply(idx); cmd != nil {
			return true, m, cmd
		}
	case 'm', 'M':
		if m.openMenuModal() {
			return true, m, nil
		}
	case 't', 'T':
		m.showCockpit = !m.showCockpit
		return true, m, nil
	case 'v', 'V':
		newM, cmd := m.startRecording()
		return true, newM, cmd
	case tea.KeySpace:
		cmd := m.toggleAudioPlayback()
		return true, m, cmd
	}
	return false, m, nil
}

func (m *model) triggerButtonReply(idx int) tea.Cmd {
	for _, msg := range slices.Backward(m.messages) {
		if msg.Direction != "inbound" {
			if msg.InteractiveContent != nil && msg.InteractiveContent.Type == domain.InteractiveTypeButton {
				if idx < len(msg.InteractiveContent.Buttons) {
					btn := msg.InteractiveContent.Buttons[idx]
					m.addOrUpdateMessage(chatMessage{
						Timestamp: time.Now(),
						From:      m.from,
						To:        m.to,
						Body:      fmt.Sprintf("You selected: %s (%s)", btn.Title, btn.ID),
						Status:    "delivered",
						Direction: "inbound",
						Engine:    domain.EngineWhatsApp,
					})
					m.status = "Sent button reply: " + btn.Title
					return sendButtonReplyCmd(m.server, m.from, m.to, btn.ID, btn.Title)
				}
			}
			if msg.TelegramMetadata != nil && len(msg.TelegramMetadata.InlineKeyboard) > 0 {
				var flatButtons []domain.ButtonItem
				for _, row := range msg.TelegramMetadata.InlineKeyboard {
					flatButtons = append(flatButtons, row...)
				}
				if idx < len(flatButtons) {
					btn := flatButtons[idx]
					m.addOrUpdateMessage(chatMessage{
						Timestamp: time.Now(),
						From:      fmt.Sprintf("%d", m.cfg.TelegramChatID),
						To:        m.to,
						Body:      fmt.Sprintf("You selected: %s", btn.Title),
						Status:    "delivered",
						Direction: "inbound",
						Engine:    domain.EngineTelegram,
					})
					m.status = "Sent inline callback: " + btn.Title
					return sendTelegramCallbackCmd(m.server, m.cfg.TelegramChatID, m.cfg.TelegramUsername, btn.ID)
				}
			}
		}
	}
	return nil
}

func (m *model) openMenuModal() bool {
	for _, msg := range slices.Backward(m.messages) {
		if msg.Direction != "inbound" && msg.InteractiveContent != nil &&
			msg.InteractiveContent.Type == domain.InteractiveTypeList {
			items := make([]menuItemOption, 0)
			for _, sec := range msg.InteractiveContent.Sections {
				for _, r := range sec.Rows {
					items = append(items, menuItemOption{
						SectionTitle: sec.Title,
						ID:           r.ID,
						Title:        r.Title,
						Description:  r.Description,
					})
				}
			}
			if len(items) > 0 {
				m.menuItems = items
				m.menuCursor = 0
				m.menuOpen = true
				return true
			}
		}
	}
	return false
}

func (m model) handleMenuKeyPress(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.Code {
	case tea.KeyUp:
		if m.menuCursor > 0 {
			m.menuCursor--
		}
		return m, nil
	case tea.KeyDown:
		if m.menuCursor < len(m.menuItems)-1 {
			m.menuCursor++
		}
		return m, nil
	case tea.KeyEscape:
		m.menuOpen = false
		return m, nil
	case tea.KeyEnter:
		if m.menuCursor >= 0 && m.menuCursor < len(m.menuItems) {
			item := m.menuItems[m.menuCursor]
			m.menuOpen = false
			m.addOrUpdateMessage(chatMessage{
				Timestamp: time.Now(),
				From:      m.from,
				To:        m.to,
				Body:      fmt.Sprintf("You selected: %s (%s)", item.Title, item.ID),
				Status:    "delivered",
				Direction: "inbound",
				Engine:    domain.EngineWhatsApp,
			})
			m.status = "Sent menu selection: " + item.Title
			return m, sendListReplyCmd(m.server, m.from, m.to, item)
		}
	}
	return m, nil
}

func (m model) startRecording() (tea.Model, tea.Cmd) {
	if m.isRecording {
		return m, nil
	}
	tmpFile, err := os.CreateTemp("", "wa_voice_*.ogg")
	if err != nil {
		m.status = "Recording error: " + err.Error()
		return m, nil
	}
	_ = tmpFile.Close()
	m.recordFile = tmpFile.Name()
	m.isRecording = true
	m.recordSeconds = 0
	m.recordVU = 0
	m.status = "Recording voice note..."

	if err := m.recorder.Start(m.ctx, m.recordFile); err != nil {
		m.isRecording = false
		_ = os.Remove(m.recordFile)
		m.status = "Mic start error: " + err.Error()
		return m, nil
	}

	return m, tickRecorder()
}

func (m model) handleRecordingKeyPress(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.Code {
	case tea.KeyEscape:
		_ = m.recorder.Cancel()
		_ = os.Remove(m.recordFile)
		m.isRecording = false
		m.status = "Recording cancelled"
		return m, nil
	case tea.KeyEnter:
		m.isRecording = false
		dur, err := m.recorder.Stop()
		m.status = "Uploading voice note..."
		return m, func() tea.Msg {
			return recordFinishedMsg{
				filePath: m.recordFile,
				duration: dur,
				err:      err,
			}
		}
	}
	return m, nil
}

func (m model) handleRecordFinished(msg recordFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = "Record error: " + msg.err.Error()
		_ = os.Remove(msg.filePath)
		return m, nil
	}
	eng := domain.EngineWhatsApp
	from := m.from
	if m.target == targetTelegram {
		eng = domain.EngineTelegram
		from = fmt.Sprintf("%d", m.cfg.TelegramChatID)
	}
	m.addOrUpdateMessage(chatMessage{
		Timestamp: time.Now(),
		From:      from,
		To:        m.to,
		Body:      fmt.Sprintf("[Voice Note] (%.1fs)", msg.duration.Seconds()),
		Status:    "delivered",
		Direction: "inbound",
		Engine:    eng,
		VoiceMetadata: &domain.VoiceMetadata{
			DurationSeconds: msg.duration.Seconds(),
			IsVoiceNote:     true,
		},
	})
	return m, sendVoiceNoteCmd(m.cfg, msg.filePath, msg.duration, m.target)
}

func (m *model) toggleAudioPlayback() tea.Cmd {
	if m.isPlaying {
		_ = m.player.Stop()
		m.isPlaying = false
		m.status = "Playback stopped"
		return nil
	}

	for _, msg := range slices.Backward(m.messages) {
		if len(msg.MediaURLs) > 0 || msg.VoiceMetadata != nil {
			target := ""
			if len(msg.MediaURLs) > 0 {
				target = msg.MediaURLs[0]
			}
			if target != "" {
				m.isPlaying = true
				m.playingSID = msg.SID
				m.status = "Playing audio..."
				return playAudioCmd(m.ctx, m.player, m.server, target)
			}
		}
	}
	m.status = "No audio message found"
	return nil
}

func (m model) handleSendResult(res sendResultMsg) (tea.Model, tea.Cmd) {
	if res.err != nil {
		m.status = "Send error: " + res.err.Error()
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].Direction == "inbound" && m.messages[i].Status == "sending" {
				m.messages[i].Status = "failed"
				break
			}
		}
	} else if res.reply != "" {
		m.status = "Ready"
		m.addOrUpdateMessage(chatMessage{
			Timestamp: time.Now(),
			From:      m.to,
			To:        m.from,
			Body:      res.reply,
			Status:    "delivered",
			Direction: "outbound-reply",
			Engine:    domain.EngineTwilio,
		})
	}
	return m, nil
}

func tickRecorder() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return recordTickMsg{t: t}
	})
}
