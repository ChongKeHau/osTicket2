package attachment

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
)

func TestLocalStorageRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := NewLocalStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	size, sum, err := st.Put(ctx, "abc123", strings.NewReader("hello"))
	if err != nil || size != 5 || sum != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatalf("put: %d %s %v", size, sum, err)
	}
	rc, err := st.Open(ctx, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "hello" {
		t.Fatalf("read back %q", b)
	}
	if err := st.Delete(ctx, "abc123"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Open(ctx, "abc123"); err == nil {
		t.Fatal("open after delete must fail")
	}
	if _, _, err := st.Put(ctx, "../escape", strings.NewReader("x")); err == nil {
		t.Fatal("keys with path separators must be rejected")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
}
