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
			t.Errorf("missing %q in output. output=%q", want, result.Output)
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

func TestConvertHeadingRequiresSpace(t *testing.T) {
	input := "**** 対策\n* Heading\n******* Long"
	result := Convert(input, ConvertOptions{Force: true})
	if !strings.Contains(result.Output, "#### 対策") {
		t.Fatalf("expected heading to convert with matching count: %q", result.Output)
	}
	if !strings.Contains(result.Output, "# Heading") {
		t.Fatalf("expected heading to convert: %q", result.Output)
	}
	if !strings.Contains(result.Output, "####### Long") {
		t.Fatalf("expected long heading to convert: %q", result.Output)
	}
}

func TestConvertHeadingAboveSixHashes(t *testing.T) {
	input := "******** Title"
	result := Convert(input, ConvertOptions{Force: true})
	if result.Output != "######## Title" {
		t.Fatalf("expected heading to convert with same count, got: %q", result.Output)
	}
}

func TestConvertQuoteWithCodeBlock(t *testing.T) {
	input := "{quote}\n{code}\nline\n{/code}\n{/quote}"
	result := Convert(input, ConvertOptions{Force: true})
	want := strings.Join([]string{
		"> {code}",
		"> line",
		"> {/code}",
	}, "\n")
	if !strings.Contains(result.Output, want) {
		t.Fatalf("expected quoted content to stay unconverted, got: %q", result.Output)
	}
}

func TestConvertSkipsQuoteLine(t *testing.T) {
	input := "> [[Title>https://example.com]]\n>{code}\n>line\n>{/code}"
	result := Convert(input, ConvertOptions{Force: true})
	if result.Output != input {
		t.Fatalf("expected quote lines to remain unchanged, got: %q", result.Output)
	}
}

func TestConvertTableAddsSeparator(t *testing.T) {
	input := strings.Join([]string{
		"| 日時 | イベント |",
		"|03/04|test|",
	}, "\n")
	result := Convert(input, ConvertOptions{Force: true})
	want := strings.Join([]string{
		"| 日時 | イベント |",
		"| --- | --- |",
		"| 03/04 | test |",
	}, "\n")
	if result.Output != want {
		t.Fatalf("expected table separator, got: %q", result.Output)
	}
}

func TestConvertTableSkipsGFMTable(t *testing.T) {
	input := strings.Join([]string{
		"| A |B|",
		"| --- | --- |",
		"|1|2|",
	}, "\n")
	result := Convert(input, ConvertOptions{Force: true})
	if result.Output != input {
		t.Fatalf("expected GFM table to remain unchanged, got: %q", result.Output)
	}
}

func TestConvertListDashSkipsDoubleDash(t *testing.T) {
	input := strings.Join([]string{
		"--- t3.large 2 台 (0.0835 USD/台)",
		"-item",
	}, "\n")
	result := Convert(input, ConvertOptions{Force: true})
	if !strings.Contains(result.Output, "    - t3.large 2 台 (0.0835 USD/台)") {
		t.Fatalf("expected nested dash list conversion: %q", result.Output)
	}
	if !strings.Contains(result.Output, "- item") {
		t.Fatalf("expected dash list without space to convert: %q", result.Output)
	}
}

func TestConvertDashListAddsBlankLine(t *testing.T) {
	input := strings.Join([]string{
		"- item",
		"-- nested",
		"reference",
	}, "\n")
	result := Convert(input, ConvertOptions{Force: true})
	want := strings.Join([]string{
		"- item",
		"  - nested",
		"",
		"reference",
	}, "\n")
	if result.Output != want {
		t.Fatalf("expected blank line after list, got: %q", result.Output)
	}
}

func TestConvertInlineCodeBlock(t *testing.T) {
	input := "ID {code}db-123{/code} test"
	result := Convert(input, ConvertOptions{Force: true})
	want := "ID `db-123` test"
	if result.Output != want {
		t.Fatalf("expected inline code conversion, got: %q", result.Output)
	}
}

func TestConvertInlineCodeBlockWithLang(t *testing.T) {
	input := "ID {code:sql}select 1{/code} test"
	result := Convert(input, ConvertOptions{Force: true})
	want := "ID `sql: select 1` test"
	if result.Output != want {
		t.Fatalf("expected inline code conversion with lang, got: %q", result.Output)
	}
}

func TestConvertImageMacroAttachment(t *testing.T) {
	input := "#image(logo.png)"
	result := Convert(input, ConvertOptions{Force: true, AttachmentNames: []string{"logo.png"}})
	if result.Output != "![image][logo.png]" {
		t.Fatalf("expected attachment image conversion, got: %q", result.Output)
	}
}

func TestConvertImageMacroURL(t *testing.T) {
	input := "#image(https://example.com/logo.png)"
	result := Convert(input, ConvertOptions{Force: true})
	if result.Output != "![image](https://example.com/logo.png)" {
		t.Fatalf("expected URL image conversion, got: %q", result.Output)
	}
}
