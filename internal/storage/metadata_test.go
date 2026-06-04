package storage

import (
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeMetadataValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain ASCII is unchanged",
			input: "Brooklyn East IPA",
			want:  "Brooklyn East IPA",
		},
		{
			name:  "accented characters are percent-encoded",
			input: "La Bête",
			want:  "La B%C3%AAte",
		},
		{
			name:  "newlines and tabs collapse to spaces",
			input: "line one\nline two\ttabbed",
			want:  "line one line two tabbed",
		},
		{
			name:  "carriage returns collapse to spaces",
			input: "a\r\nb",
			want:  "a  b",
		},
		{
			name:  "other control characters are stripped",
			input: "a\x00\x07b",
			want:  "ab",
		},
		{
			name:  "surrounding whitespace is trimmed",
			input: "  spaced  ",
			want:  "spaced",
		},
		{
			name:  "percent is encoded so decoding is reversible",
			input: "100%",
			want:  "100%25",
		},
		{
			name:  "emoji is percent-encoded",
			input: "beer 🍺",
			want:  "beer %F0%9F%8D%BA",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sanitizeMetadataValue(tt.input))
		})
	}
}

func TestSanitizeMetadataValue_HeaderSafe(t *testing.T) {
	// every output byte must be printable single-line US-ASCII.
	inputs := []string{"La Bête à Biere", "multi\nline\tvalue", "emoji 🍺", "100%"}
	for _, in := range inputs {
		out := sanitizeMetadataValue(in)
		for i := 0; i < len(out); i++ {
			c := out[i]
			if c < 0x20 || c > 0x7e {
				t.Errorf("non-printable byte %#x in output %q for input %q", c, out, in)
			}
		}
	}
}

func TestSanitizeMetadataValue_RoundTrip(t *testing.T) {
	inputs := []string{"La Bête à Biere", "100% great", "beer 🍺 time", "Brooklyn Brewery"}
	for _, in := range inputs {
		decoded, err := url.PathUnescape(sanitizeMetadataValue(in))
		require.NoError(t, err)
		assert.Equal(t, in, decoded)
	}
}

func TestSanitizeMetadataValue_TruncatesOnRuneBoundary(t *testing.T) {
	// "世" is three bytes; 200 of them exceed the 512 byte cap and the cut
	// lands mid-rune, so it must trim back to a whole rune (170 * 3 = 510).
	out := sanitizeMetadataValue(strings.Repeat("世", 200))

	decoded, err := url.PathUnescape(out)
	require.NoError(t, err)
	assert.True(t, utf8.ValidString(decoded), "decoded value must be valid UTF-8")
	assert.Equal(t, strings.Repeat("世", 170), decoded)
}

func TestCheckinMetadata_ToMap_Sanitizes(t *testing.T) {
	md := &CheckinMetadata{
		Beer:    "La Bête",
		Comment: "great\nbeer",
		ID:      "123",
	}

	m := md.ToMap()
	assert.Equal(t, "La B%C3%AAte", m["beer"])
	assert.Equal(t, "great beer", m["comment"])
	assert.Equal(t, "123", m["id"])
}
