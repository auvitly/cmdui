package ui

import (
	"strings"
	"testing"
)

func TestLinkifyDescriptionLinksHTTPURLsAndEscapesOtherText(t *testing.T) {
	got := string(linkifyDescription(`Документация: https://example.com/docs?q=one&v=two. <script>alert(1)</script>`))
	if !strings.Contains(got, `<a href="https://example.com/docs?q=one&amp;v=two" target="_blank" rel="noopener noreferrer">https://example.com/docs?q=one&amp;v=two</a>.`) {
		t.Fatalf("http link was not safely rendered: %s", got)
	}
	if strings.Contains(got, "<script>") || !strings.Contains(got, "&lt;script&gt;") {
		t.Fatalf("description HTML was not escaped: %s", got)
	}
}

func TestLinkifyDescriptionDoesNotLinkUnsupportedSchemes(t *testing.T) {
	got := string(linkifyDescription(`javascript:alert(1) ftp://example.com/file`))
	if strings.Contains(got, "<a ") {
		t.Fatalf("unsupported scheme became clickable: %s", got)
	}
}
