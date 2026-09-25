package importer

import (
	"bytes"
	"context"
	"regexp"
	"testing"
)

func TestMapEntryTypeAndFormat(t *testing.T) {
	for in, want := range map[string]string{"M": "message", "R": "response", "N": "note"} {
		if got, ok := mapEntryType(in); !ok || got != want {
			t.Errorf("%s = %s,%v", in, got, ok)
		}
	}
	if _, ok := mapEntryType("X"); ok {
		t.Error("X must not map")
	}
	if mapFormat("text") != "text" || mapFormat("html") != "html" || mapFormat("markdown") != "html" || mapFormat("") != "html" {
		t.Error("format mapping")
	}
}

func TestStorageKeyAndDiscard(t *testing.T) {
	k, err := newStorageKey()
	if err != nil || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(k) {
		t.Fatal(k, err)
	}
	var d discardStorage
	n, sum, err := d.Put(context.Background(), k, bytes.NewReader([]byte("hello world")))
	if err != nil || n != 11 || sum != "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9" {
		t.Fatal(n, sum, err)
	}
	if _, err := d.Open(context.Background(), k); err == nil {
		t.Fatal("discard storage cannot open")
	}
}

func TestOpenSourceFileInvalidKey(t *testing.T) {
	rc, reason, err := openSourceFile(context.Background(), nil, SrcFile{Backend: "F", Key: "../etc/passwd"}, t.TempDir())
	if err != nil || reason != "invalid file key" || rc != nil {
		t.Fatalf("rc=%v reason=%q err=%v", rc, reason, err)
	}
}
