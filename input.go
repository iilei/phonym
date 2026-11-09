package phonym

import "fmt"

// INPUT RULE (frozen; widening it later would make decoders disagree):
//  1. Format characters are removed. They mark nothing the user typed, so they
//     never separate words.
//  2. Separators are the code points in separators; runs count as one, leading
//     and trailing ones are ignored.
//  3. ASCII letters are lowercased; afterwards only a-z may remain. Everything
//     else is rejected: '.' and '+' (reserved), URL and path characters,
//     apostrophes, diacritics and look-alike letters (never folded).
//
// The tables are explicit code point lists (generated once from Unicode 16.0
// whitespace and dash punctuation), so results never depend on the Unicode
// version of a runtime.

var (
	// formatChars are removed before splitting.
	formatChars = map[rune]bool{
		0x00AD: true, // SOFT HYPHEN
		0x200B: true, // ZERO WIDTH SPACE
		0x200C: true, // ZERO WIDTH NON-JOINER
		0x200D: true, // ZERO WIDTH JOINER
		0xFEFF: true, // ZERO WIDTH NO-BREAK SPACE (BOM)
	}

	// separators split words: whitespace, dash punctuation, minus sign, low line, middle dot.
	separators = map[rune]bool{
		0x0009:  true, // CHARACTER TABULATION
		0x000A:  true, // LINE FEED
		0x000B:  true, // LINE TABULATION
		0x000C:  true, // FORM FEED
		0x000D:  true, // CARRIAGE RETURN
		0x001C:  true, // INFORMATION SEPARATOR FOUR
		0x001D:  true, // INFORMATION SEPARATOR THREE
		0x001E:  true, // INFORMATION SEPARATOR TWO
		0x001F:  true, // INFORMATION SEPARATOR ONE
		0x0020:  true, // SPACE
		0x002D:  true, // HYPHEN-MINUS
		0x005F:  true, // LOW LINE
		0x0085:  true, // NEXT LINE
		0x00A0:  true, // NO-BREAK SPACE
		0x00B7:  true, // MIDDLE DOT
		0x058A:  true, // ARMENIAN HYPHEN
		0x05BE:  true, // HEBREW PUNCTUATION MAQAF
		0x1400:  true, // CANADIAN SYLLABICS HYPHEN
		0x1680:  true, // OGHAM SPACE MARK
		0x1806:  true, // MONGOLIAN TODO SOFT HYPHEN
		0x2000:  true, // EN QUAD
		0x2001:  true, // EM QUAD
		0x2002:  true, // EN SPACE
		0x2003:  true, // EM SPACE
		0x2004:  true, // THREE-PER-EM SPACE
		0x2005:  true, // FOUR-PER-EM SPACE
		0x2006:  true, // SIX-PER-EM SPACE
		0x2007:  true, // FIGURE SPACE
		0x2008:  true, // PUNCTUATION SPACE
		0x2009:  true, // THIN SPACE
		0x200A:  true, // HAIR SPACE
		0x2010:  true, // HYPHEN
		0x2011:  true, // NON-BREAKING HYPHEN
		0x2012:  true, // FIGURE DASH
		0x2013:  true, // EN DASH
		0x2014:  true, // EM DASH
		0x2015:  true, // HORIZONTAL BAR
		0x2028:  true, // LINE SEPARATOR
		0x2029:  true, // PARAGRAPH SEPARATOR
		0x202F:  true, // NARROW NO-BREAK SPACE
		0x205F:  true, // MEDIUM MATHEMATICAL SPACE
		0x2212:  true, // MINUS SIGN
		0x2E17:  true, // DOUBLE OBLIQUE HYPHEN
		0x2E1A:  true, // HYPHEN WITH DIAERESIS
		0x2E3A:  true, // TWO-EM DASH
		0x2E3B:  true, // THREE-EM DASH
		0x2E40:  true, // DOUBLE HYPHEN
		0x2E5D:  true, // OBLIQUE HYPHEN
		0x3000:  true, // IDEOGRAPHIC SPACE
		0x301C:  true, // WAVE DASH
		0x3030:  true, // WAVY DASH
		0x30A0:  true, // KATAKANA-HIRAGANA DOUBLE HYPHEN
		0xFE31:  true, // PRESENTATION FORM FOR VERTICAL EM DASH
		0xFE32:  true, // PRESENTATION FORM FOR VERTICAL EN DASH
		0xFE58:  true, // SMALL EM DASH
		0xFE63:  true, // SMALL HYPHEN-MINUS
		0xFF0D:  true, // FULLWIDTH HYPHEN-MINUS
		0x10D6E: true, // GARAY HYPHEN
		0x10EAD: true, // YEZIDI HYPHENATION MARK
	}
)

// normalize splits a name into lowercase a-z words according to the INPUT RULE.
func normalize(name string) ([]string, error) {
	var words []string
	var word []byte
	for _, r := range name {
		switch {
		case formatChars[r]:
			continue
		case separators[r]:
			if len(word) > 0 {
				words = append(words, string(word))
				word = word[:0]
			}
			continue
		case r >= 'A' && r <= 'Z':
			r += 'a' - 'A'
		}
		if r < 'a' || r > 'z' {
			what := "not allowed"
			if r == '.' || r == '+' {
				what = "reserved"
			}
			return nil, fmt.Errorf("%w: %q (U+%04X) %s", ErrInvalidName, r, r, what)
		}
		word = append(word, byte(r))
	}
	if len(word) > 0 {
		words = append(words, string(word))
	}
	if len(words) == 0 {
		return nil, fmt.Errorf("%w: empty name", ErrInvalidName)
	}
	return words, nil
}
