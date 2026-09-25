package inbound

import "testing"

func TestCleanSubjectAndNumber(t *testing.T) {
	cases := map[string]string{
		"Printer on fire":                      "Printer on fire",
		"Re: [#100001] Printer on fire":        "Printer on fire",
		"FW: Fwd: Re: RE: [#100002] forwarded": "forwarded",
		"  ":                                   "(no subject)",
		"[#7] ":                                "(no subject)",
		"Re:Re: no space":                      "no space",
	}
	for in, want := range cases {
		if got := CleanSubject(in); got != want {
			t.Errorf("CleanSubject(%q) = %q want %q", in, got, want)
		}
	}
	if NumberFromSubject("Re: [#100001] x") != "100001" || NumberFromSubject("no tag") != "" || NumberFromSubject("[#100001-5] dup") != "100001-5" {
		t.Fatal("NumberFromSubject")
	}
	ids := TicketIDsFromRefs([]string{"<x@y.test>", "<ticket-3-abc@d.test>", "<ticket-3-abc@d.test>", "<ticket-9-def@d.test>"})
	if len(ids) != 2 || ids[0] != 3 || ids[1] != 9 {
		t.Fatalf("ids = %v", ids)
	}
}
