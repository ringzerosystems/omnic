package domain_test

import (
	"testing"

	"github.com/ringzerosystems/omnic/internal/domain"
)

func TestCalculateSegments(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantEncoding domain.EncodingType
		wantSegments int
		hasMedia     bool
	}{
		{
			name:         "empty body without media",
			body:         "",
			hasMedia:     false,
			wantSegments: 1,
			wantEncoding: domain.EncodingGSM7,
		},
		{
			name:         "short GSM-7 message",
			body:         "Hello world!",
			hasMedia:     false,
			wantSegments: 1,
			wantEncoding: domain.EncodingGSM7,
		},
		{
			name:         "exact 160 chars GSM-7 message",
			body:         string(make([]rune, 160)),
			hasMedia:     false,
			wantSegments: 1,
			wantEncoding: domain.EncodingGSM7,
		},
		{
			name:         "161 chars GSM-7 message",
			body:         string(make([]rune, 161)),
			hasMedia:     false,
			wantSegments: 2,
			wantEncoding: domain.EncodingGSM7,
		},
		{
			name:         "GSM-7 extension character counts as 2",
			body:         "Price: 50€", // '€' is an extension char: 9 base chars + 1 ext char (weight 2) = 11
			hasMedia:     false,
			wantSegments: 1,
			wantEncoding: domain.EncodingGSM7,
		},
		{
			name:         "Unicode emoji triggers UCS-2",
			body:         "Hello 👋",
			hasMedia:     false,
			wantSegments: 1,
			wantEncoding: domain.EncodingUCS2,
		},
		{
			name:         "70 chars UCS-2 message",
			body:         "👋" + string(make([]rune, 68)), // 69 chars with emoji
			hasMedia:     false,
			wantSegments: 1,
			wantEncoding: domain.EncodingUCS2,
		},
		{
			name:         "71 chars UCS-2 message",
			body:         "👋" + string(make([]rune, 70)), // 71 chars with emoji
			hasMedia:     false,
			wantSegments: 2,
			wantEncoding: domain.EncodingUCS2,
		},
		{
			name:         "MMS with media always has 1 segment",
			body:         "A long message with media " + string(make([]rune, 300)),
			hasMedia:     true,
			wantSegments: 1,
			wantEncoding: domain.EncodingGSM7,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// For tests using make([]rune, n), fill with 'a' so it's valid GSM-7 (except where emoji was prepended)
			body := tc.body
			if len(body) > 0 && body[0] == 0 {
				runes := []rune(body)
				for i := range runes {
					if runes[i] == 0 {
						runes[i] = 'a'
					}
				}
				body = string(runes)
			} else if len(body) > 4 && body[4] == 0 {
				runes := []rune(body)
				for i := range runes {
					if runes[i] == 0 {
						runes[i] = 'a'
					}
				}
				body = string(runes)
			}

			segments, enc := domain.CalculateSegments(body, tc.hasMedia)
			if segments != tc.wantSegments {
				t.Errorf("CalculateSegments() segments = %d, want %d", segments, tc.wantSegments)
			}
			if enc != tc.wantEncoding {
				t.Errorf("CalculateSegments() encoding = %v, want %v", enc, tc.wantEncoding)
			}
		})
	}
}
