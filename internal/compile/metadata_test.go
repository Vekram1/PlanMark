package compile

import (
	"reflect"
	"testing"
)

func TestMetadataParse(t *testing.T) {
	src := []byte("- [ ] Item\n  @id t1\n  @horizon now\n  @deps a,b\n  @accept cmd:go test ./...\n  @foo custom opaque\n")

	parsed, err := ParseMetadata(src)
	if err != nil {
		t.Fatalf("ParseMetadata returned error: %v", err)
	}

	if len(parsed.KnownByKey["id"]) != 1 || parsed.KnownByKey["id"][0].Value != "t1" {
		t.Fatalf("expected known id metadata entry, got %#v", parsed.KnownByKey["id"])
	}
	if len(parsed.KnownByKey["horizon"]) != 1 || parsed.KnownByKey["horizon"][0].Value != "now" {
		t.Fatalf("expected known horizon metadata entry, got %#v", parsed.KnownByKey["horizon"])
	}
	if len(parsed.KnownByKey["deps"]) != 1 || parsed.KnownByKey["deps"][0].Value != "a,b" {
		t.Fatalf("expected known deps metadata entry, got %#v", parsed.KnownByKey["deps"])
	}
	if len(parsed.KnownByKey["accept"]) != 1 {
		t.Fatalf("expected known accept metadata entry, got %#v", parsed.KnownByKey["accept"])
	}

	wantOpaque := []MetadataEntry{{Key: "foo", Value: "custom opaque", Line: 6}}
	if !reflect.DeepEqual(parsed.Opaque, wantOpaque) {
		t.Fatalf("opaque metadata mismatch\nwant=%#v\ngot=%#v", wantOpaque, parsed.Opaque)
	}
}

func TestMetadataParseIgnoresFencedBlocks(t *testing.T) {
	src := []byte("```md\n@id should.not.parse\n@foo hidden\n```\n- [ ] task\n@id real.id\n")
	parsed, err := ParseMetadata(src)
	if err != nil {
		t.Fatalf("ParseMetadata returned error: %v", err)
	}
	if got := len(parsed.All); got != 1 {
		t.Fatalf("expected 1 metadata entry outside fences, got %d (%#v)", got, parsed.All)
	}
	if parsed.All[0].Key != "id" || parsed.All[0].Value != "real.id" {
		t.Fatalf("unexpected parsed entry: %#v", parsed.All[0])
	}
}
