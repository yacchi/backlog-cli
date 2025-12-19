package auth

import "testing"

func TestIsJapanesePreferred(t *testing.T) {
	tests := []struct {
		name           string
		acceptLanguage string
		want           bool
	}{
		{
			name:           "Japanese first",
			acceptLanguage: "ja,en-US;q=0.9,en;q=0.8",
			want:           true,
		},
		{
			name:           "Japanese with region first",
			acceptLanguage: "ja-JP,ja;q=0.9,en;q=0.8",
			want:           true,
		},
		{
			name:           "English first",
			acceptLanguage: "en-US,en;q=0.9,ja;q=0.8",
			want:           false,
		},
		{
			name:           "Japanese only",
			acceptLanguage: "ja",
			want:           true,
		},
		{
			name:           "English only",
			acceptLanguage: "en",
			want:           false,
		},
		{
			name:           "Empty",
			acceptLanguage: "",
			want:           false,
		},
		{
			name:           "Complex with Japanese first",
			acceptLanguage: "ja;q=1.0, en-US;q=0.9, en;q=0.8",
			want:           true,
		},
		{
			name:           "Other language first",
			acceptLanguage: "fr-FR,fr;q=0.9,en;q=0.8,ja;q=0.7",
			want:           false,
		},
		{
			name:           "Japanese uppercase",
			acceptLanguage: "JA,en;q=0.8",
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isJapanesePreferred(tt.acceptLanguage)
			if got != tt.want {
				t.Errorf("isJapanesePreferred(%q) = %v, want %v", tt.acceptLanguage, got, tt.want)
			}
		})
	}
}

func TestExtractLanguageTag(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ja", "ja"},
		{"ja-JP", "ja-jp"},
		{"en-US;q=0.9", "en-us"},
		{"JA;q=1.0", "ja"},
		{"EN", "en"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractLanguageTag(tt.input)
			if got != tt.want {
				t.Errorf("extractLanguageTag(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
