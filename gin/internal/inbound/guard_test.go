package inbound

import "testing"

func TestIgnoreReason(t *testing.T) {
	ok := &Parsed{FromAddress: "pat@example.test", ContentType: "text/plain"}
	if r := IgnoreReason(ok, "desk@example.test"); r != "" {
		t.Fatalf("normal mail ignored: %q", r)
	}
	cases := []struct {
		name string
		p    Parsed
	}{
		{"own address", Parsed{FromAddress: "Desk@Example.test"}},
		{"auto-submitted", Parsed{FromAddress: "a@b.test", AutoSubmitted: "auto-generated"}},
		{"precedence bulk", Parsed{FromAddress: "a@b.test", Precedence: "bulk"}},
		{"precedence list", Parsed{FromAddress: "a@b.test", Precedence: "List"}},
		{"suppress", Parsed{FromAddress: "a@b.test", AutoResponseSuppress: "OOF, AutoReply"}},
		{"mailer-daemon", Parsed{FromAddress: "MAILER-DAEMON@mx.test"}},
		{"postmaster", Parsed{FromAddress: "postmaster@mx.test"}},
		{"noreply", Parsed{FromAddress: "no-reply@shop.test"}},
		{"report", Parsed{FromAddress: "a@b.test", ContentType: "multipart/report; report-type=delivery-status"}},
	}
	for _, c := range cases {
		if r := IgnoreReason(&c.p, "desk@example.test"); r == "" {
			t.Errorf("%s: not ignored", c.name)
		}
	}
	if r := IgnoreReason(&Parsed{FromAddress: "a@b.test", AutoSubmitted: "no"}, "desk@example.test"); r != "" {
		t.Fatalf("Auto-Submitted: no is normal mail, got %q", r)
	}
}
