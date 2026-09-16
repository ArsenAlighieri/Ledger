package handlers

import (
	"testing"

	"ledger/web"
)

func TestNewRenderer(t *testing.T) {
	renderer, err := NewRenderer(web.TemplateFS())
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}
	if renderer.templates == nil {
		t.Fatal("expected templates to not be nil")
	}
}
