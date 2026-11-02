package domain

// EncodingType represents the SMS character encoding.
type EncodingType string

const (
	EncodingGSM7 EncodingType = "GSM-7"
	EncodingUCS2 EncodingType = "UCS-2"
)

// gsm7Base contains the basic GSM 03.38 character set.
var gsm7Base = map[rune]struct{}{
	'@': {}, '£': {}, '$': {}, '¥': {}, 'è': {}, 'é': {}, 'ù': {}, 'ì': {},
	'ò': {}, 'Ç': {}, '\n': {}, 'Ø': {}, 'ø': {}, '\r': {}, 'Å': {}, 'å': {},
	'Δ': {}, '_': {}, 'Φ': {}, 'Γ': {}, 'Λ': {}, 'Ω': {}, 'Π': {}, 'Ψ': {},
	'Σ': {}, 'Θ': {}, 'Ξ': {}, 'Æ': {}, 'æ': {}, 'ß': {}, 'É': {}, ' ': {},
	'!': {}, '"': {}, '#': {}, '¤': {}, '%': {}, '&': {}, '\'': {}, '(': {},
	')': {}, '*': {}, '+': {}, ',': {}, '-': {}, '.': {}, '/': {}, '0': {},
	'1': {}, '2': {}, '3': {}, '4': {}, '5': {}, '6': {}, '7': {}, '8': {},
	'9': {}, ':': {}, ';': {}, '<': {}, '=': {}, '>': {}, '?': {}, '¡': {},
	'A': {}, 'B': {}, 'C': {}, 'D': {}, 'E': {}, 'F': {}, 'G': {}, 'H': {},
	'I': {}, 'J': {}, 'K': {}, 'L': {}, 'M': {}, 'N': {}, 'O': {}, 'P': {},
	'Q': {}, 'R': {}, 'S': {}, 'T': {}, 'U': {}, 'V': {}, 'W': {}, 'X': {},
	'Y': {}, 'Z': {}, 'Ä': {}, 'Ö': {}, 'Ñ': {}, 'Ü': {}, '§': {}, '¿': {},
	'a': {}, 'b': {}, 'c': {}, 'd': {}, 'e': {}, 'f': {}, 'g': {}, 'h': {},
	'i': {}, 'j': {}, 'k': {}, 'l': {}, 'm': {}, 'n': {}, 'o': {}, 'p': {},
	'q': {}, 'r': {}, 's': {}, 't': {}, 'u': {}, 'v': {}, 'w': {}, 'x': {},
	'y': {}, 'z': {}, 'ä': {}, 'ö': {}, 'ñ': {}, 'ü': {}, 'à': {},
}

// gsm7Ext contains the GSM 03.38 extension characters (each counts as 2 units).
var gsm7Ext = map[rune]struct{}{
	'^': {}, '{': {}, '}': {}, '\\': {}, '[': {}, '~': {}, ']': {}, '|': {}, '€': {},
}

// CalculateSegments calculates the SMS/MMS segment count and character encoding.
func CalculateSegments(body string, hasMedia bool) (int, EncodingType) {
	if hasMedia {
		return 1, EncodingGSM7
	}
	if body == "" {
		return 1, EncodingGSM7
	}

	units := 0
	isGSM := true
	charCount := 0

	for _, r := range body {
		charCount++
		if _, ok := gsm7Ext[r]; ok {
			units += 2
			continue
		}
		if _, ok := gsm7Base[r]; ok {
			units++
			continue
		}
		isGSM = false
	}

	if !isGSM {
		if charCount <= 70 {
			return 1, EncodingUCS2
		}
		return (charCount + 66) / 67, EncodingUCS2
	}

	if units <= 160 {
		return 1, EncodingGSM7
	}
	return (units + 152) / 153, EncodingGSM7
}
