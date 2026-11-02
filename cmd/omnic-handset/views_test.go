package main

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ringzerosystems/omnic/internal/domain"
)

func TestRenderPanelBoundaryIntegrity(t *testing.T) {
	testCases := []struct {
		name   string
		width  int
		height int
	}{
		{"Small Panel", 30, 8},
		{"Standard Panel", 80, 24},
		{"Wide Panel", 120, 30},
		{"Narrow Panel", 24, 6},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			content := "Short line\n" +
				cyanTextStyle.Render("Very long styled line that exceeds panel dimensions significantly by a lot") +
				"\nNormal text\nAnother line with special unicode ✓ ● ■"

			rendered := renderPanel("Test Panel", content, tc.width, tc.height, true)
			lines := strings.Split(rendered, "\n")
			if len(lines) != tc.height {
				t.Fatalf("expected %d vertical lines, got %d", tc.height, len(lines))
			}

			for i, line := range lines {
				w := lipgloss.Width(line)
				if w != tc.width {
					t.Errorf("line %d visual width = %d, expected exactly %d: %q", i, w, tc.width, line)
				}
			}
		})
	}
}

func TestComputeLayout(t *testing.T) {
	cases := []struct {
		w, h       int
		compact    bool
		restricted bool
	}{
		{80, 24, false, false},
		{120, 40, false, false},
		{60, 24, true, false},
		{100, 15, false, true},
		{50, 12, true, true},
	}

	for _, tc := range cases {
		geom := computeLayout(tc.w, tc.h)
		if geom.totalWidth != tc.w || geom.totalHeight != tc.h {
			t.Errorf("size %dx%d mismatch", tc.w, tc.h)
		}
		if geom.isCompact != tc.compact {
			t.Errorf("%dx%d compact want %v got %v", tc.w, tc.h, tc.compact, geom.isCompact)
		}
		if geom.isRestricted != tc.restricted {
			t.Errorf("%dx%d restricted want %v got %v", tc.w, tc.h, tc.restricted, geom.isRestricted)
		}
		if !tc.compact && (geom.leftWidth+geom.rightWidth != tc.w) {
			t.Errorf("split width mismatch: %d + %d != %d", geom.leftWidth, geom.rightWidth, tc.w)
		}
	}
}

func TestModernizedViewsAndCockpit(t *testing.T) {
	m := newPhoneModel(phoneConfig{
		Server: "http://localhost:8080",
		From:   "+14155551111",
		To:     "+14155550199",
		Engine: "all",
	})
	m.width = 100
	m.height = 30

	m.addOrUpdateMessage(chatMessage{
		Timestamp: time.Now(),
		From:      "+14155551111",
		To:        "+14155550199",
		Body:      "Hello from phone client",
		Status:    "delivered",
		Direction: "inbound",
		Engine:    domain.EngineWhatsApp,
	})

	view := m.View()
	content := view.Content

	if !strings.Contains(content, "MOBILE HANDSET") {
		t.Errorf("expected header to contain MOBILE HANDSET")
	}
	if !strings.Contains(content, "HANDSET COCKPIT") {
		t.Errorf("expected view to contain HANDSET COCKPIT in standard mode")
	}
	if !strings.Contains(content, "CONVERSATION THREAD") {
		t.Errorf("expected view to contain CONVERSATION THREAD")
	}
	if !strings.Contains(content, "COMPOSER") {
		t.Errorf("expected view to contain COMPOSER")
	}
}

func TestCompactModePaneToggle(t *testing.T) {
	m := newPhoneModel(phoneConfig{
		Server: "http://localhost:8080",
		From:   "+14155551111",
		To:     "+14155550199",
		Engine: "whatsapp",
	})
	m.width = 60
	m.height = 24

	// Initially show conversation thread
	view1 := m.View().Content
	if !strings.Contains(view1, "CONVERSATION THREAD") {
		t.Errorf("expected conversation thread initially in compact mode")
	}

	// Press 't' to toggle cockpit
	m2, _ := m.Update(tea.KeyPressMsg{Code: 't'})
	curM := m2.(model)
	if !curM.showCockpit {
		t.Errorf("expected showCockpit to be true after pressing 't'")
	}
	view2 := curM.View().Content
	if !strings.Contains(view2, "HANDSET COCKPIT") {
		t.Errorf("expected HANDSET COCKPIT after pressing 't' in compact mode")
	}

	// Press 't' again to toggle back
	m3, _ := curM.Update(tea.KeyPressMsg{Code: 't'})
	if m3.(model).showCockpit {
		t.Errorf("expected showCockpit to be false after second 't'")
	}
}

func TestFormatBytesAndTruncate(t *testing.T) {
	if got := formatBytes(500); got != "500 B" {
		t.Errorf("expected 500 B, got %q", got)
	}
	if got := formatBytes(2048); got != "2.0 KB" {
		t.Errorf("expected 2.0 KB, got %q", got)
	}

	if got := truncateString("", 5); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	if got := truncateString("hello", 2); got != "he" {
		t.Errorf("expected 'he', got %q", got)
	}
	if got := truncateString("hello world", 8); got != "hello..." {
		t.Errorf("expected 'hello...', got %q", got)
	}
}
