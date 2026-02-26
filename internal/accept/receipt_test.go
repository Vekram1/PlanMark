package accept

import "testing"

func TestParseCommandText(t *testing.T) {
	cmd, err := ParseCommandText("cmd:go test ./...")
	if err != nil {
		t.Fatalf("parse command: %v", err)
	}
	if cmd != "go test ./..." {
		t.Fatalf("unexpected command text: %q", cmd)
	}
}

func TestParseCommandTextRejectsNonCmdPrefix(t *testing.T) {
	if _, err := ParseCommandText("file:README.md"); err == nil {
		t.Fatalf("expected parse error for non-cmd accept format")
	}
}

func TestNewCaptureStreamIncludesDigestWhenPayloadPresent(t *testing.T) {
	stream := NewCaptureStream([]byte("hello"))
	if !stream.Captured {
		t.Fatalf("expected captured=true")
	}
	if stream.Bytes != 5 {
		t.Fatalf("expected bytes=5, got %d", stream.Bytes)
	}
	if stream.Digest == nil || stream.Digest.Value == "" {
		t.Fatalf("expected digest for non-empty stream")
	}
}
