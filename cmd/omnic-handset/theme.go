package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Tokyo Night Truecolor Palette Tokens.
var (
	colorSurfaceBg       = lipgloss.Color("#161822") // Deep Charcoal Navy
	colorSurfaceElevated = lipgloss.Color("#1e2130") // Midnight Blue
	colorBorderInactive  = lipgloss.Color("#32384e") // Muted Slate Blue
	colorBorderActive    = lipgloss.Color("#7aa2f7") // Electric Sky Blue
	colorBorderSubtle    = lipgloss.Color("#232738") // Subtle Navy
	colorPrimary         = lipgloss.Color("#7aa2f7") // Electric Sky Blue
	colorSecondary       = lipgloss.Color("#bb9af7") // Lavender / Soft Purple
	colorAccentCyan      = lipgloss.Color("#2ac3de") // Vivid Cyan
	colorSuccess         = lipgloss.Color("#9ece6a") // Emerald Green
	colorWarning         = lipgloss.Color("#e0af68") // Warm Amber
	colorError           = lipgloss.Color("#f7768e") // Coral Red
	colorTextNormal      = lipgloss.Color("#c0caf5") // Pale Periwinkle
	colorTextMuted       = lipgloss.Color("#565f89") // Cool Gray Slate
	colorTextInverted    = lipgloss.Color("#0f1117") // Dark Ink
	colorPureWhite       = lipgloss.Color("#ffffff")
)

// Standard Unicode Glyphs.
const (
	symCheck     = "✓"
	symCheckRead = "✓✓"
	symCross     = "✗"
	symRunning   = "●"
	symIdle      = "○"
	symDryRun    = "✦"
	symCursor    = "❯"
	symTab       = "▶"
	symArrow     = "→"
	symSkipped   = "↷"
	symBlockFull = "█"
	symBlock75   = "▓"
	symBlock50   = "▒"
	symBlock25   = "░"
	symBlockDot  = "·"
)

// Pre-allocated Lipgloss styles for zero-allocation rendering in View() loops.
var (
	textNormalStyle  = lipgloss.NewStyle().Foreground(colorTextNormal)
	textMutedStyle   = lipgloss.NewStyle().Foreground(colorTextMuted)
	primaryTextStyle = lipgloss.NewStyle().Foreground(colorPrimary)
	cyanTextStyle    = lipgloss.NewStyle().Foreground(colorAccentCyan)
	warningTextStyle = lipgloss.NewStyle().Foreground(colorWarning)
	errorTextStyle   = lipgloss.NewStyle().Foreground(colorError)
	boldTextStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorTextNormal)
	boldPrimaryStyle = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)

	agentLabelStyle       = lipgloss.NewStyle().Bold(true).Foreground(colorSecondary)
	telegramBotLabelStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccentCyan)

	activeHeaderTag = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorTextInverted).
			Background(colorPrimary).
			Padding(0, 1)

	inactiveHeaderTag = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorTextNormal).
				Background(colorBorderInactive).
				Padding(0, 1)

	successBadge = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorTextInverted).
			Background(colorSuccess).
			Padding(0, 1)

	warningBadge = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorTextInverted).
			Background(colorWarning).
			Padding(0, 1)

	errorBadge = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPureWhite).
			Background(colorError).
			Padding(0, 1)

	cyanBadge = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorTextInverted).
			Background(colorAccentCyan).
			Padding(0, 1)

	badgeTwilio = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorTextInverted).
			Background(colorPrimary).
			Padding(0, 1)

	badgeWhatsApp = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorTextInverted).
			Background(colorSuccess).
			Padding(0, 1)

	badgeTelegram = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorTextInverted).
			Background(colorAccentCyan).
			Padding(0, 1)

	borderActiveStyle   = lipgloss.NewStyle().Foreground(colorBorderActive)
	borderInactiveStyle = lipgloss.NewStyle().Foreground(colorBorderInactive)
	borderSubtleStyle   = lipgloss.NewStyle().Foreground(colorBorderSubtle)

	keyPillStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorTextInverted).
			Background(colorPrimary).
			Padding(0, 1)

	keyDescStyle = lipgloss.NewStyle().Foreground(colorTextMuted)

	buttonPillStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorTextInverted).
			Background(colorPrimary).
			Padding(0, 1)

	modalSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorTextInverted).
				Background(colorPrimary).
				Padding(0, 1)

	inputPromptStyle = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	inputTextStyle   = lipgloss.NewStyle().Foreground(colorTextNormal)
	inputCursorStyle = lipgloss.NewStyle().Foreground(colorPrimary)

	userBubbleTwilioStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorPrimary).
				Background(colorSurfaceBg).
				Padding(0, 1)

	userBubbleWhatsAppStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorSuccess).
				Background(colorSurfaceBg).
				Padding(0, 1)

	userBubbleTelegramStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorAccentCyan).
				Background(colorSurfaceBg).
				Padding(0, 1)

	agentBubbleTwilioStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorSecondary).
				Background(colorSurfaceElevated).
				Padding(0, 1)

	agentBubbleWhatsAppStyle = lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(colorSuccess).
					Background(colorSurfaceElevated).
					Padding(0, 1)

	agentBubbleTelegramStyle = lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(colorAccentCyan).
					Background(colorSurfaceElevated).
					Padding(0, 1)
)

// renderPanel constructs a bounded panel with embedded title pill and zero-wrap guarantee.
func renderPanel(title, content string, width, height int, active bool) string {
	if width < 10 {
		width = 10
	}
	if height < 3 {
		height = 3
	}

	bStyle := borderInactiveStyle
	titleFormatted := inactiveHeaderTag.Render(title)
	if active {
		bStyle = borderActiveStyle
		titleFormatted = activeHeaderTag.Render(title)
	}

	titleW := lipgloss.Width(titleFormatted)
	remaining := max(width-titleW-5, 0)
	topLine := bStyle.Render("╭─") + " " + titleFormatted + " " + bStyle.Render(strings.Repeat("─", remaining)+"╮")

	innerW := max(width-4, 1)
	innerH := max(height-2, 1)

	rawLines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	middleLines := make([]string, 0, innerH)
	for i := range innerH {
		line := ""
		if i < len(rawLines) {
			line = rawLines[i]
		}
		if lipgloss.Width(line) > innerW {
			line = ansi.Truncate(line, innerW, "")
		}
		pad := max(innerW-lipgloss.Width(line), 0)
		padded := line + strings.Repeat(" ", pad)
		middleLines = append(middleLines, bStyle.Render("│")+" "+padded+" "+bStyle.Render("│"))
	}

	bottomLine := bStyle.Render("╰" + strings.Repeat("─", width-2) + "╯")

	var sb strings.Builder
	sb.Grow(width * height * 2)
	sb.WriteString(topLine)
	sb.WriteByte('\n')
	for _, m := range middleLines {
		sb.WriteString(m)
		sb.WriteByte('\n')
	}
	sb.WriteString(bottomLine)

	return sb.String()
}

// truncateString truncates ANSI-aware text to maxLen.
func truncateString(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if maxLen <= 3 {
		return ansi.Truncate(s, maxLen, "")
	}
	return ansi.Truncate(s, maxLen, "...")
}

// formatBytes returns a formatted byte size string.
func formatBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	return fmt.Sprintf("%.1f KB", float64(b)/1024.0)
}
