package markdown

import (
	"strings"
	"testing"
)

func TestConvertBacklogToGFM(t *testing.T) {
	input := "* Title\n{quote}Quoted\nLine{/quote}\n{code:go}\nfmt.Println(\"x\")\n{/code}\n[[Label>https://example.com]]\n''bold'' '''italic''' %%strike%%\n#contents\n&br;\n+ item"

	result := Convert(input, ConvertOptions{})
	if result.Output == input {
		t.Fatalf("expected conversion to change output")
	}

	if result.Mode != ModeBacklog {
		t.Fatalf("expected backlog mode, got %s", result.Mode)
	}

	checks := []string{
		"# Title",
		"> Quoted",
		"```go",
		"[Label](https://example.com)",
		"**bold**",
		"*italic*",
		"~~strike~~",
		"[toc]",
		"<br>",
		"1. item",
	}
	for _, want := range checks {
		if !strings.Contains(result.Output, want) {
			t.Errorf("missing %q in output", want)
		}
	}
}

func TestConvertSkipsInlineCode(t *testing.T) {
	input := "`''bold''`"
	result := Convert(input, ConvertOptions{Force: true})
	if result.Output != input {
		t.Fatalf("expected inline code to remain unchanged")
	}
}

func TestConvertSkipsGFM(t *testing.T) {
	input := "```\ncode\n```\n- [x] done"
	result := Convert(input, ConvertOptions{})
	if result.Mode != ModeMarkdown {
		t.Fatalf("expected markdown mode, got %s", result.Mode)
	}
	if result.Output != input {
		t.Fatalf("expected GFM content to remain unchanged")
	}
}
