package main

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	headerHeight  = 3
	footerHeight  = 4
	minTermWidth  = 80
	minTermHeight = 24
)

type layoutGeometry struct {
	totalWidth   int
	totalHeight  int
	headerHeight int
	footerHeight int
	bodyHeight   int
	leftWidth    int
	rightWidth   int
	isCompact    bool
	isRestricted bool
}

func computeLayout(width, height int) layoutGeometry {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	isRestricted := height < 20
	isCompact := width < minTermWidth

	hHeight := headerHeight
	if isRestricted {
		hHeight = 2
	}
	fHeight := footerHeight
	bodyHeight := max(height-hHeight-fHeight, 6)

	leftWidth := (width * 58) / 100
	rightWidth := width - leftWidth
	if isCompact {
		leftWidth = width
		rightWidth = width
	}

	return layoutGeometry{
		totalWidth:   width,
		totalHeight:  height,
		headerHeight: hHeight,
		footerHeight: fHeight,
		bodyHeight:   bodyHeight,
		leftWidth:    leftWidth,
		rightWidth:   rightWidth,
		isCompact:    isCompact,
		isRestricted: isRestricted,
	}
}

func renderHeader(m *model, geom layoutGeometry) string {
	width := geom.totalWidth
	if geom.isRestricted {
		return renderRestrictedHeader(m, width)
	}

	var engineBadge string
	switch m.configuredEngine {
	case "whatsapp":
		engineBadge = badgeWhatsApp.Render("💬 WhatsApp")
	case "telegram":
		engineBadge = badgeTelegram.Render("✈️ Telegram")
	case "all":
		engineBadge = successBadge.Render("⚡ Tri-Engine")
	default:
		engineBadge = badgeTwilio.Render("📱 Twilio")
	}

	onlineBadge := successBadge.Render("● ONLINE")
	titlePart := activeHeaderTag.Render("📱 MOBILE HANDSET")
	leftSegment := borderInactiveStyle.Render("╭─ ") + titlePart + " "
	leftW := lipgloss.Width(leftSegment)
	remain := max(width-leftW-1, 0)
	topLine := leftSegment + borderInactiveStyle.Render(strings.Repeat("─", remain)+"╮")

	innerW := max(width-4, 1)

	leftText := boldTextStyle.Render("Handset: ") + cyanTextStyle.Render(m.from) +
		textMutedStyle.Render(" │ ") + boldTextStyle.Render("Recipient: ") + cyanTextStyle.Render(m.to) +
		textMutedStyle.Render(" (AI Support Agent)") +
		textMutedStyle.Render(" │ ") + boldTextStyle.Render("Server: ") + textMutedStyle.Render(m.server) +
		textMutedStyle.Render(" │ ") + engineBadge

	space := innerW - lipgloss.Width(leftText) - lipgloss.Width(onlineBadge)
	var midContent string
	if space > 0 {
		midContent = leftText + strings.Repeat(" ", space) + onlineBadge
	} else {
		midContent = leftText
	}

	if lipgloss.Width(midContent) > innerW {
		midContent = ansi.Truncate(midContent, innerW, "")
	}
	pad := max(innerW-lipgloss.Width(midContent), 0)
	midLine := borderInactiveStyle.Render("│") + " " + midContent + strings.Repeat(" ", pad) + " " + borderInactiveStyle.Render("│")
	botLine := borderInactiveStyle.Render("╰" + strings.Repeat("─", width-2) + "╯")

	return topLine + "\n" + midLine + "\n" + botLine
}

func renderRestrictedHeader(m *model, width int) string {
	var engineBadge string
	switch m.configuredEngine {
	case "whatsapp":
		engineBadge = badgeWhatsApp.Render("💬 WA")
	case "telegram":
		engineBadge = badgeTelegram.Render("✈️ TG")
	case "all":
		engineBadge = successBadge.Render("⚡ ALL")
	default:
		engineBadge = badgeTwilio.Render("📱 SMS")
	}

	line1 := boldPrimaryStyle.Render("📱 Handset: ") + cyanTextStyle.Render(m.from) +
		textMutedStyle.Render(" │ ") + boldTextStyle.Render("To: ") + cyanTextStyle.Render(m.to) +
		textMutedStyle.Render(" (AI Support Agent)") +
		textMutedStyle.Render(" │ ") + engineBadge + " " + successBadge.Render("● ONLINE")
	if lipgloss.Width(line1) > width {
		line1 = ansi.Truncate(line1, width, "")
	}
	line2 := borderSubtleStyle.Render(strings.Repeat("─", max(width, 1)))
	return line1 + "\n" + line2
}

func renderFooter(m *model, geom layoutGeometry) string {
	width := geom.totalWidth
	topLine := borderInactiveStyle.Render("╭" + strings.Repeat("─", width-2) + "╮")

	keys := []string{
		keyPillStyle.Render("Enter") + " " + keyDescStyle.Render("Send"),
		keyPillStyle.Render("Tab") + " " + keyDescStyle.Render("Target"),
		keyPillStyle.Render("V") + " " + keyDescStyle.Render("Voice"),
		keyPillStyle.Render("Space") + " " + keyDescStyle.Render("Play"),
		keyPillStyle.Render("1-3") + " " + keyDescStyle.Render("Button"),
		keyPillStyle.Render("M") + " " + keyDescStyle.Render("Menu"),
		keyPillStyle.Render("↑/↓") + " " + keyDescStyle.Render("Scroll"),
	}
	if geom.isCompact {
		keys = append(keys, keyPillStyle.Render("T")+" "+keyDescStyle.Render("Pane"))
	}
	keys = append(keys, keyPillStyle.Render("Esc")+" "+keyDescStyle.Render("Quit"))

	hotkeyStr := strings.Join(keys, "  ")
	hotkeyLine := renderFooterRow(hotkeyStr, width, borderInactiveStyle)

	var statusStr string
	switch {
	case m.status == "Ready":
		statusStr = successBadge.Render("● Ready") + "  " + textMutedStyle.Render("Listening on SSE events")
	case strings.Contains(strings.ToLower(m.status), "thinking"):
		statusStr = warningBadge.Render("● ⚡ "+m.status) + " " + textMutedStyle.Render("(AI Agent processing)")
	case strings.Contains(strings.ToLower(m.status), "error"):
		statusStr = errorBadge.Render("✗ " + m.status)
	case strings.Contains(strings.ToLower(m.status), "recording"):
		statusStr = errorBadge.Render("🔴 " + m.status)
	case strings.Contains(strings.ToLower(m.status), "playing"):
		statusStr = cyanBadge.Render("🔊 " + m.status)
	default:
		statusStr = cyanBadge.Render("●") + " " + textNormalStyle.Render(m.status)
	}

	statusLine := renderFooterRow(statusStr, width, borderInactiveStyle)
	bottomLine := borderInactiveStyle.Render("╰" + strings.Repeat("─", width-2) + "╯")

	return topLine + "\n" + hotkeyLine + "\n" + statusLine + "\n" + bottomLine
}

func renderFooterRow(content string, width int, borderStyle lipgloss.Style) string {
	innerW := max(width-4, 1)
	if lipgloss.Width(content) > innerW {
		content = ansi.Truncate(content, innerW, "")
	}
	pad := max(innerW-lipgloss.Width(content), 0)
	return borderStyle.Render("│") + " " + content + strings.Repeat(" ", pad) + " " + borderStyle.Render("│")
}
