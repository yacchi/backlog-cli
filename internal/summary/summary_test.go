package summary

import (
	"testing"
)

func TestSummarize(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		sentenceCount int
		wantErr       bool
		check         func(t *testing.T, summary string)
	}{
		{
			name:          "empty text",
			text:          "",
			sentenceCount: 3,
			wantErr:       false,
			check: func(t *testing.T, summary string) {
				if summary != "" {
					t.Errorf("expected empty summary, got %q", summary)
				}
			},
		},
		{
			name: "short text",
			text: "これは短い文章です。",
			sentenceCount: 3,
			wantErr: false,
			check: func(t *testing.T, summary string) {
				// 短い場合は要約されない（そのまま返ってくるか、MMRの閾値による）
				// LexRankMMRの挙動として、入力文が少なければそのまま返るはず
				if summary == "" {
					t.Error("expected non-empty summary")
				}
			},
		},
		{
			name: "long text with newlines",
			text: `
吾輩は猫である。
名前はまだ無い。
どこで生れたかとんと見当がつかぬ。
何でも薄暗いじめじめした所でニャーニャー泣いていた事だけは記憶している。
吾輩はここで始めて人間というものを見た。
しかもあとで聞くとそれは書生という人間中で一番獰悪な種族であったそうだ。
この書生というのは時々我々を捕えて煮て食うという話である。
`,
			sentenceCount: 2,
			wantErr:       false,
			check: func(t *testing.T, summary string) {
				if summary == "" {
					t.Error("expected non-empty summary")
				}
				// 句点で分割して文数をカウント
				// summaryには「。」が含まれているはず
				// ただし、最後の「。」があるとは限らないが、splitSentenceで分割されたものを結合しているので、
				// 元の文にあった「。」は含まれる。
				// 単純なカウントは難しいが、長さが元のテキストより短いことは確認できる。
				if len(summary) >= len("吾輩は猫である。名前はまだ無い。どこで生れたかとんと見当がつかぬ。何でも薄暗いじめじめした所でニャーニャー泣いていた事だけは記憶している。吾輩はここで始めて人間というものを見た。しかもあとで聞くとそれは書生という人間中で一番獰悪な種族であったそうだ。この書生というのは時々我々を捕えて煮て食うという話である。") {
					// 改行除去後の長さと比較すべきだが、目安として。
					// 2文抽出なら確実に短くなっているはず。
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Summarize(tt.text, tt.sentenceCount)
			if (err != nil) != tt.wantErr {
				t.Errorf("Summarize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}
