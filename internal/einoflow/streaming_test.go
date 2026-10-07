package einoflow

import (
	"strings"
	"testing"
)

func TestVisibleTextExtractorHandlesArbitraryJSONAndUnicodeFragments(t *testing.T) {
	extractor := newVisibleTextExtractor()
	parts := []string{
		`{"analysis":"SECRET","nested":{"text":"HIDDEN"},"claims":[{"fragment_ids":["f1"],"text":"A\`,
		`n中\uD83D`,
		`\uDE00"},{"text":"B"}],"gaps":["NOT_VISIBLE"]}`,
	}
	var visible strings.Builder
	for _, part := range parts {
		visible.WriteString(extractor.Write(part))
	}
	if got, want := visible.String(), "A\n中😀\n\nB"; got != want {
		t.Fatalf("visible text mismatch: %q, want %q", got, want)
	}
	for _, secret := range []string{"SECRET", "HIDDEN", "NOT_VISIBLE", "fragment_ids", "claims"} {
		if strings.Contains(visible.String(), secret) {
			t.Fatalf("internal field leaked: %q", secret)
		}
	}
}

func TestVisibleTextExtractorRejectsMalformedEscapeWithoutLeakingFollowingFields(t *testing.T) {
	extractor := newVisibleTextExtractor()
	first := extractor.Write(`{"claims":[{"text":"safe\uD83D`)
	second := extractor.Write(`x"}],"analysis":"LEAK"}`)
	if first != "safe" || second != "" {
		t.Fatalf("malformed stream was exposed: first=%q second=%q", first, second)
	}
}

func TestVisibleTextExtractorOnlyUsesDirectRootClaims(t *testing.T) {
	extractor := newVisibleTextExtractor()
	got := extractor.Write(`{"wrapper":{"claims":[{"text":"nested"}]},"claims":[{"metadata":{"text":"inner"},"text":"shown"}]}`)
	if got != "shown" {
		t.Fatalf("unexpected path extraction: %q", got)
	}
}

func TestVisibleTextExtractorReplyAcrossEveryByteBoundary(t *testing.T) {
	raw := `{"wrapper":{"reply":{"text":"HIDDEN"}},"reply":{"kind":"INTRODUCTION","metadata":{"text":"HIDDEN"},"text":"你好\n\uD83D\uDE00"},"claims":[],"gaps":["HIDDEN"],"analysis":"HIDDEN"}`
	for split := 0; split <= len(raw); split++ {
		extractor := newVisibleTextExtractor()
		got := extractor.Write(raw[:split]) + extractor.Write(raw[split:])
		if got != "你好\n😀" {
			t.Fatalf("split %d: reply text mismatch: %q", split, got)
		}
	}
}
