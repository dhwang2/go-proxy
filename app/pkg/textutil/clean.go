// Package textutil strips terminal control sequences from text this program
// did not write: another program's log, a unit name, an error message.
//
// It lives outside internal/cli because the guarantee is not a display one.
// `--json` never carries escape sequences under any condition, so the stripping
// has to happen where the foreign text enters, not where a human rendering
// happens to colour it.
package textutil

import (
	"strings"
	"unicode/utf8"
)

// Clean removes escape sequences and C0 control characters, newlines included.
// Use it for a value that occupies one field of a rendered row, where an
// embedded newline would break the layout as surely as an escape would break
// the colour guarantee.
func Clean(value string) string { return strip(value, false) }

// CleanText is Clean for content that is a document rather than a field: line
// breaks survive, every other control character does not. A log read back is
// the whole reason this exists -- stripping its newlines would leave one line.
func CleanText(value string) string { return strip(value, true) }

func strip(value string, keepNewlines bool) string {
	if strings.IndexFunc(value, func(r rune) bool { return control(r) && !(keepNewlines && r == '\n') }) < 0 {
		return value
	}
	var out strings.Builder
	out.Grow(len(value))
	for index := 0; index < len(value); {
		char := value[index]
		if char == 0x1b {
			index += escapeLength(value[index:])
			continue
		}
		if control(rune(char)) && char < utf8.RuneSelf && !(keepNewlines && char == '\n') {
			index++
			continue
		}
		size := 1
		if char >= utf8.RuneSelf {
			_, size = utf8.DecodeRuneInString(value[index:])
		}
		out.WriteString(value[index : index+size])
		index += size
	}
	return out.String()
}

// escapeLength measures the escape sequence starting at value[0], which is an
// ESC. A CSI sequence runs to its final byte in 0x40-0x7e; anything else is one
// escape plus at most one character, which is enough to drop the short forms
// without swallowing the text after a stray ESC.
func escapeLength(value string) int {
	if len(value) < 2 {
		return len(value)
	}
	if value[1] != '[' {
		return 2
	}
	for index := 2; index < len(value); index++ {
		if value[index] >= 0x40 && value[index] <= 0x7e {
			return index + 1
		}
		if value[index] < 0x20 || value[index] > 0x3f {
			return index
		}
	}
	return len(value)
}

func control(r rune) bool { return r < 0x20 || r == 0x7f }
