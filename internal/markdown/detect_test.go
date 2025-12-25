package markdown

import "testing"

func TestDetectBacklog(t *testing.T) {
	input := "{code}fmt.Println(\"x\"){/code}"
	result := Detect(input)
	if result.Mode != ModeBacklog {
		t.Fatalf("expected backlog mode, got %s", result.Mode)
	}
	if result.Score < 2 {
		t.Fatalf("expected score >= 2, got %d", result.Score)
	}
}

func TestDetectBacklogImageMacro(t *testing.T) {
	input := "#image(https://example.com/image.png)"
	result := Detect(input)
	if result.Mode != ModeBacklog {
		t.Fatalf("expected backlog mode, got %s", result.Mode)
	}
	if result.Score < 2 {
		t.Fatalf("expected score >= 2, got %d", result.Score)
	}
}

func TestDetectBacklogHeadingAsterisk(t *testing.T) {
	input := "*** Heading"
	result := Detect(input)
	if result.Mode != ModeBacklog {
		t.Fatalf("expected backlog mode, got %s", result.Mode)
	}
	if result.Score < 2 {
		t.Fatalf("expected score >= 2, got %d", result.Score)
	}
}

func TestDetectBacklogHeadingWithTildes(t *testing.T) {
	input := "*** Heading\n~~~ [details] ~~~"
	result := Detect(input)
	if result.Mode != ModeBacklog {
		t.Fatalf("expected backlog mode, got %s", result.Mode)
	}
	if result.Score < 2 {
		t.Fatalf("expected score >= 2, got %d", result.Score)
	}
}

func TestDetectGFM(t *testing.T) {
	input := "```\ncode\n```\n- [x] done"
	result := Detect(input)
	if result.Mode != ModeMarkdown {
		t.Fatalf("expected markdown mode, got %s", result.Mode)
	}
	if result.Score > -1 {
		t.Fatalf("expected score <= -1, got %d", result.Score)
	}
}
