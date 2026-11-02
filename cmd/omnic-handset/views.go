package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ringzerosystems/omnic/internal/domain"
)

func (m model) View() tea.View {
	geom := computeLayout(m.width, m.height)
	header := renderHeader(&m, geom)
	footer := renderFooter(&m, geom)

	var body string
	switch {
	case m.menuOpen:
		body = renderMenuModal(&m, geom.totalWidth, geom.bodyHeight)
	case m.isRecording:
		body = renderRecordingHUD(&m, geom.totalWidth, geom.bodyHeight)
	case geom.isCompact:
		body = renderCompactLayout(&m, geom)
	default:
		body = renderStandardLayout(&m, geom)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	return tea.NewView(content)
}

func renderStandardLayout(m *model, geom layoutGeometry) string {
	threadH := max(geom.bodyHeight-3, 3)
	threadPanel := renderConversationThread(m, geom.leftWidth, threadH)
	composerPanel := renderComposer(m, geom.leftWidth)
	leftCol := lipgloss.JoinVertical(lipgloss.Left, threadPanel, composerPanel)

	cockpitPanel := renderHandsetCockpit(m, geom.rightWidth, geom.bodyHeight)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftCol, cockpitPanel)
}

func renderCompactLayout(m *model, geom layoutGeometry) string {
	if m.showCockpit {
		return renderHandsetCockpit(m, geom.totalWidth, geom.bodyHeight)
	}
	threadH := max(geom.bodyHeight-3, 3)
	threadPanel := renderConversationThread(m, geom.totalWidth, threadH)
	composerPanel := renderComposer(m, geom.totalWidth)
	return lipgloss.JoinVertical(lipgloss.Left, threadPanel, composerPanel)
}

func renderConversationThread(m *model, width, height int) string {
	innerW := max(width-4, 1)
	innerH := max(height-2, 1)

	if len(m.messages) == 0 {
		placeholder := textMutedStyle.Render("(No messages yet. Send a message to start conversation)")
		padTop := max((innerH-1)/2, 0)
		var sb strings.Builder
		for range padTop {
			sb.WriteByte('\n')
		}
		sb.WriteString(lipgloss.PlaceHorizontal(innerW, lipgloss.Center, placeholder))
		return renderPanel("💬 CONVERSATION THREAD", sb.String(), width, height, true)
	}

	allLines := make([]string, 0, len(m.messages)*4)
	for i, msg := range m.messages {
		var block string
		if msg.From == m.from || msg.Direction == "inbound" {
			block = renderUserMessage(msg, innerW)
		} else {
			block = renderAgentMessage(msg, innerW)
		}
		allLines = append(allLines, strings.Split(block, "\n")...)
		if i < len(m.messages)-1 {
			allLines = append(allLines, "")
		}
	}

	visibleLines := sliceVisibleLines(allLines, innerH, m.scrollOffset)
	content := strings.Join(visibleLines, "\n")
	title := "💬 CONVERSATION THREAD"
	if m.scrollOffset > 0 {
		title = fmt.Sprintf("💬 CONVERSATION (▲ +%d)", m.scrollOffset)
	}
	return renderPanel(title, content, width, height, true)
}

func sliceVisibleLines(lines []string, availHeight, scrollOffset int) []string {
	if lines == nil {
		lines = []string{}
	}
	total := len(lines)
	if total <= availHeight {
		res := make([]string, 0, availHeight)
		res = append(res, lines...)
		for len(res) < availHeight {
			res = append(res, "")
		}
		return res
	}

	maxScroll := total - availHeight
	scroll := max(0, min(scrollOffset, maxScroll))
	end := total - scroll
	start := end - availHeight
	return lines[start:end]
}

func renderComposer(m *model, width int) string {
	var targetBadge string
	switch m.target {
	case targetTwilio:
		targetBadge = badgeTwilio.Render("📱 Twilio")
	case targetWhatsApp:
		targetBadge = badgeWhatsApp.Render("💬 WhatsApp")
	case targetTelegram:
		targetBadge = badgeTelegram.Render("✈️ Telegram")
	case targetAll:
		targetBadge = successBadge.Render("⚡ Broadcast (All)")
	default:
		targetBadge = badgeWhatsApp.Render("💬 WhatsApp")
	}

	title := "COMPOSER " + targetBadge
	prompt := inputPromptStyle.Render("❯ ")
	cursor := inputCursorStyle.Render("█")
	inputText := inputTextStyle.Render(m.input)
	if m.input == "" {
		inputText = textMutedStyle.Render("Type message... (Tab to cycle)")
	}
	content := prompt + inputText + cursor
	return renderPanel(title, content, width, 3, true)
}

func renderUserMessage(msg chatMessage, maxInnerW int) string {
	timeStr := textMutedStyle.Render("[" + msg.Timestamp.Format("15:04") + "]")
	badge := renderEngineBadge(msg.Engine)
	userStr := boldPrimaryStyle.Render("You")
	statusStr := formatStatusGlyph(msg.Status)
	meta := timeStr + " " + userStr + " " + badge + " " + statusStr

	bubbleWidth := max(min(maxInnerW-6, 54), 22)
	body := renderMessageBody(msg, bubbleWidth-4)
	w := max(min(lipgloss.Width(body)+4, bubbleWidth), 18)

	bubbleStyle := userBubbleTwilioStyle
	switch msg.Engine {
	case domain.EngineWhatsApp:
		bubbleStyle = userBubbleWhatsAppStyle
	case domain.EngineTelegram:
		bubbleStyle = userBubbleTelegramStyle
	}

	bubble := bubbleStyle.Width(w).Render(body)
	block := lipgloss.JoinVertical(lipgloss.Right, meta, bubble)
	return lipgloss.PlaceHorizontal(maxInnerW, lipgloss.Right, block)
}

func renderAgentMessage(msg chatMessage, maxInnerW int) string {
	timeStr := textMutedStyle.Render("[" + msg.Timestamp.Format("15:04") + "]")
	badge := renderEngineBadge(msg.Engine)
	agentStr := agentLabelStyle.Render("🤖 AI Support Agent:")
	if msg.Engine == domain.EngineTelegram {
		agentStr = telegramBotLabelStyle.Render("✈️ Telegram Bot:")
	}
	meta := timeStr + " " + agentStr + " " + badge

	bubbleWidth := max(min(maxInnerW-6, 54), 22)
	body := renderMessageBody(msg, bubbleWidth-4)
	w := max(min(lipgloss.Width(body)+4, bubbleWidth), 18)

	bubbleStyle := agentBubbleTwilioStyle
	switch msg.Engine {
	case domain.EngineWhatsApp:
		bubbleStyle = agentBubbleWhatsAppStyle
	case domain.EngineTelegram:
		bubbleStyle = agentBubbleTelegramStyle
	}

	bubble := bubbleStyle.Width(w).Render(body)
	block := lipgloss.JoinVertical(lipgloss.Left, meta, bubble)
	return lipgloss.PlaceHorizontal(maxInnerW, lipgloss.Left, block)
}

func renderEngineBadge(eng domain.Engine) string {
	switch eng {
	case domain.EngineWhatsApp:
		return badgeWhatsApp.Render("💬 WhatsApp")
	case domain.EngineTelegram:
		return badgeTelegram.Render("✈️ Telegram")
	default:
		return badgeTwilio.Render("📱 Twilio")
	}
}

func formatStatusGlyph(status string) string {
	switch strings.ToLower(status) {
	case "read":
		return cyanTextStyle.Render(symCheckRead + " Read")
	case "delivered":
		return textMutedStyle.Render(symCheckRead)
	case "sent":
		return textMutedStyle.Render(symCheck)
	case "failed", "undelivered":
		return errorTextStyle.Render(symCross + " Failed")
	case "sending", "queued":
		return warningTextStyle.Render(symRunning + " Sending")
	default:
		if status == "" {
			return ""
		}
		return textMutedStyle.Render("(" + status + ")")
	}
}

func renderMessageBody(msg chatMessage, maxBodyW int) string {
	var parts []string
	if msg.VoiceMetadata != nil || msg.Body == "[Voice Note]" {
		durStr := "0:10"
		if msg.VoiceMetadata != nil && msg.VoiceMetadata.DurationSeconds > 0 {
			durStr = fmt.Sprintf("0:%02d", int(msg.VoiceMetadata.DurationSeconds))
		}
		var waveBar string
		if msg.VoiceMetadata != nil && len(msg.VoiceMetadata.Waveform) > 0 {
			glyphs := []rune{' ', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
			var b strings.Builder
			for _, v := range msg.VoiceMetadata.Waveform {
				idx := min(max(v/4, 0), len(glyphs)-1)
				b.WriteRune(glyphs[idx])
			}
			waveBar = b.String()
		} else {
			waveBar = " ▃▅▇█▇▅▃ ▂▃▅▇█▇▅▃ "
		}
		card := fmt.Sprintf("🎤 Voice Note (%s)\n[▶ Space: Play]  %s", durStr, cyanTextStyle.Render(waveBar))
		parts = append(parts, card)
	} else if msg.Body != "" {
		wrapped := wrapText(msg.Body, maxBodyW)
		parts = append(parts, strings.Join(wrapped, "\n"))
	}

	if msg.InteractiveContent != nil {
		ic := msg.InteractiveContent
		if len(ic.Buttons) > 0 {
			var btnPills []string
			for i, b := range ic.Buttons {
				pill := buttonPillStyle.Render(fmt.Sprintf("%d: %s", i+1, b.Title))
				btnPills = append(btnPills, pill)
			}
			parts = append(parts, strings.Join(btnPills, "  "))
		}
		if ic.Type == domain.InteractiveTypeList {
			menuPrompt := keyPillStyle.Render("Press 'M' to Open Menu")
			parts = append(parts, menuPrompt)
		}
	}

	if msg.TelegramMetadata != nil {
		tm := msg.TelegramMetadata
		if len(tm.InlineKeyboard) > 0 {
			count := 1
			for _, row := range tm.InlineKeyboard {
				var rowPills []string
				for _, b := range row {
					pill := buttonPillStyle.Render(fmt.Sprintf("%d: %s", count, b.Title))
					rowPills = append(rowPills, pill)
					count++
				}
				parts = append(parts, strings.Join(rowPills, "  "))
			}
		}
	}

	return strings.Join(parts, "\n\n")
}

func wrapText(s string, maxW int) []string {
	if maxW <= 10 {
		return strings.Split(s, "\n")
	}
	var out []string
	lines := strings.SplitSeq(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for l := range lines {
		if lipgloss.Width(l) <= maxW {
			out = append(out, l)
			continue
		}
		words := strings.Fields(l)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		var cur strings.Builder
		for _, w := range words {
			switch {
			case cur.Len() == 0:
				cur.WriteString(w)
			case lipgloss.Width(cur.String())+1+lipgloss.Width(w) <= maxW:
				cur.WriteByte(' ')
				cur.WriteString(w)
			default:
				out = append(out, cur.String())
				cur.Reset()
				cur.WriteString(w)
			}
		}
		if cur.Len() > 0 {
			out = append(out, cur.String())
		}
	}
	return out
}

func renderHandsetCockpit(m *model, width, height int) string {
	innerW := max(width-4, 1)

	var targetBadge string
	switch m.target {
	case targetTwilio:
		targetBadge = badgeTwilio.Render("📱 Twilio")
	case targetWhatsApp:
		targetBadge = badgeWhatsApp.Render("💬 WhatsApp")
	case targetTelegram:
		targetBadge = badgeTelegram.Render("✈️ Telegram")
	case targetAll:
		targetBadge = successBadge.Render("⚡ Tri-Engine")
	default:
		targetBadge = badgeWhatsApp.Render("💬 WhatsApp")
	}

	playStatus := textMutedStyle.Render("○ Idle")
	if m.isPlaying {
		playStatus = cyanBadge.Render("● Playing")
	}

	var sb strings.Builder
	sb.Grow(width * height * 2)

	sb.WriteString(boldPrimaryStyle.Render("● PROTOCOL & ROUTE"))
	sb.WriteByte('\n')
	writeCockpitRow(&sb, "Target:    ", targetBadge, "(Tab)")
	writeCockpitRow(&sb, "Recipient: ", cyanTextStyle.Render(truncateString(m.to, innerW-14)), "")
	writeCockpitRow(&sb, "Handset:   ", primaryTextStyle.Render(truncateString(m.from, innerW-14)), "")
	writeCockpitRow(&sb, "Server:    ", textNormalStyle.Render(truncateString(m.server, innerW-14)), "")
	sb.WriteByte('\n')

	sb.WriteString(boldPrimaryStyle.Render("● AUDIO SUBSYSTEM"))
	sb.WriteByte('\n')
	writeCockpitRow(&sb, "Input:     ", textNormalStyle.Render(truncateString(m.audioIn, innerW-14)), "")
	writeCockpitRow(&sb, "Output:    ", textNormalStyle.Render(truncateString(m.audioOut, innerW-14)), "")
	writeCockpitRow(&sb, "Audio:     ", playStatus, "(Space)")
	sb.WriteByte('\n')

	sb.WriteString(boldPrimaryStyle.Render("● SHORTCUT ACTIONS"))
	sb.WriteByte('\n')
	writeShortcutRow(&sb, "Enter", "Send Message")
	writeShortcutRow(&sb, "Tab", "Cycle Target")
	writeShortcutRow(&sb, "V", "Record Voice Note")
	writeShortcutRow(&sb, "Space", "Audio Playback")
	writeShortcutRow(&sb, "1-3", "Interactive Button")
	writeShortcutRow(&sb, "M", "Technical Menu")
	writeShortcutRow(&sb, "C-k", "Clear History")
	sb.WriteByte('\n')

	sb.WriteString(boldPrimaryStyle.Render("● SESSION TELEMETRY"))
	sb.WriteByte('\n')
	userCount, agentCount := countMessages(m.messages, m.from)
	fmt.Fprintf(&sb, "  %s %s (%d user · %d agent)\n",
		textMutedStyle.Render("Messages:  "),
		textNormalStyle.Render(fmt.Sprintf("%d total", len(m.messages))),
		userCount, agentCount)
	sb.WriteString("  ")
	sb.WriteString(textMutedStyle.Render("SSE Stream:"))
	sb.WriteByte(' ')
	sb.WriteString(successBadge.Render("● Active"))
	sb.WriteByte('\n')

	return renderPanel("⚙ HANDSET COCKPIT", sb.String(), width, height, false)
}

func writeCockpitRow(sb *strings.Builder, label, val, hint string) {
	sb.WriteString("  ")
	sb.WriteString(textMutedStyle.Render(label))
	sb.WriteString(val)
	if hint != "" {
		sb.WriteByte(' ')
		sb.WriteString(textMutedStyle.Render(hint))
	}
	sb.WriteByte('\n')
}

func writeShortcutRow(sb *strings.Builder, key, desc string) {
	sb.WriteString("  ")
	sb.WriteString(keyPillStyle.Render(key))
	sb.WriteByte(' ')
	sb.WriteString(textNormalStyle.Render(desc))
	sb.WriteByte('\n')
}

func countMessages(msgs []chatMessage, userFrom string) (int, int) {
	u, a := 0, 0
	for _, m := range msgs {
		if m.From == userFrom || m.Direction == "inbound" {
			u++
		} else {
			a++
		}
	}
	return u, a
}

func renderMenuModal(m *model, width, height int) string {
	modalW := min(max(width-8, 44), 90)
	modalH := min(max(height-4, 10), 22)

	var sb strings.Builder
	sb.Grow(modalW * modalH * 2)
	sb.WriteString(boldPrimaryStyle.Render("Technical Menu Options:"))
	sb.WriteString("\n\n")

	for i, item := range m.menuItems {
		cursor := "  "
		line := fmt.Sprintf("%d. %s", i+1, item.Title)
		if i == m.menuCursor {
			cursor = cyanTextStyle.Render(symCursor + " ")
			line = modalSelectedStyle.Render(line)
		} else {
			line = textNormalStyle.Render(line)
		}
		sb.WriteString(cursor)
		sb.WriteString(line)
		sb.WriteByte('\n')
		if item.Description != "" {
			sb.WriteString("     ")
			sb.WriteString(textMutedStyle.Render(item.Description))
			sb.WriteByte('\n')
		}
	}

	sb.WriteByte('\n')
	sb.WriteString(textMutedStyle.Render("[↑/↓] Navigate  [Enter] Select  [Esc] Close"))
	modalPanel := renderPanel("📋 Technical Menu", sb.String(), modalW, modalH, true)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modalPanel)
}

func renderRecordingHUD(m *model, width, height int) string {
	modalW := min(max(width-8, 44), 80)
	modalH := min(max(height-6, 9), 14)

	vuChars := m.recordVU
	vuBar := primaryTextStyle.Render(strings.Repeat(symBlockFull, vuChars)) +
		textMutedStyle.Render(strings.Repeat(symBlock25, max(10-vuChars, 0)))

	var sb strings.Builder
	sb.Grow(modalW * modalH * 2)
	sb.WriteString(errorBadge.Render("🔴 LIVE MICROPHONE RECORDING"))
	sb.WriteString("\n\n")
	sb.WriteString(boldTextStyle.Render("Device:   "))
	sb.WriteString(cyanTextStyle.Render(m.audioIn))
	sb.WriteByte('\n')
	sb.WriteString(boldTextStyle.Render("Duration: "))
	fmt.Fprintf(&sb, "00:%02d / 05:00     ", m.recordSeconds)
	sb.WriteString(boldTextStyle.Render("VU: "))
	sb.WriteByte('[')
	sb.WriteString(vuBar)
	sb.WriteString("]\n\n")
	sb.WriteString(keyPillStyle.Render("Enter"))
	sb.WriteByte(' ')
	sb.WriteString(textNormalStyle.Render("Send Voice Note"))
	sb.WriteString("    ")
	sb.WriteString(keyPillStyle.Render("Esc"))
	sb.WriteByte(' ')
	sb.WriteString(textNormalStyle.Render("Discard"))

	hudPanel := renderPanel("🔴 RECORDING VOICE NOTE", sb.String(), modalW, modalH, true)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, hudPanel)
}
