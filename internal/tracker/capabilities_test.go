package tracker

import "testing"

func TestBeadsAdapterCapabilities(t *testing.T) {
	adapter := NewBeadsAdapter()

	got := adapter.Capabilities()
	if got.AdapterName != "beads" {
		t.Fatalf("expected beads adapter name, got %#v", got)
	}
	if !got.Title {
		t.Fatalf("expected beads to support titles, got %#v", got)
	}
	if got.Body != TextMarkdown {
		t.Fatalf("expected markdown body support, got %#v", got)
	}
	if got.Steps != CapabilityRendered {
		t.Fatalf("expected rendered step support, got %#v", got)
	}
	if got.ChildWork != CapabilityUnsupported {
		t.Fatalf("expected no child-work support, got %#v", got)
	}
	if got.CustomFields != CapabilityUnsupported {
		t.Fatalf("expected no custom-field support, got %#v", got)
	}
	if !got.RuntimeOverlays.Status || !got.RuntimeOverlays.Assignee || !got.RuntimeOverlays.Priority {
		t.Fatalf("expected safe runtime overlays for beads, got %#v", got)
	}
	if got.ProjectionSchema != ProjectionSchemaVersionV02 {
		t.Fatalf("expected projection schema %q, got %#v", ProjectionSchemaVersionV02, got)
	}
}
