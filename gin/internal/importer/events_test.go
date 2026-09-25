package importer

import "testing"

func TestMapEvent(t *testing.T) {
	cases := []struct {
		name, data, kind, payload string
		ok                        bool
	}{
		{"created", "", "created", `{}`, true},
		{"closed", "", "closed", `{}`, true},
		{"reopened", "", "reopened", `{}`, true},
		{"transferred", `{"dept":2}`, "transferred", `{"dept":2}`, true},
		{"edited", "not json", "edited", `{"raw":"not json"}`, true},
		{"assigned", `{"staff":1}`, "assigned", `{"staff":1}`, true},
		{"assigned", `{"staff":0}`, "unassigned", `{"staff":0}`, true},
		{"assigned", ``, "unassigned", `{}`, true},
		{"assigned", `{"team":3}`, "unassigned", `{"team":3}`, true},
		{"viewed", "", "", "", false},
		{"released", "", "", "", false},
	}
	for _, c := range cases {
		kind, payload, ok := mapEvent(c.name, c.data)
		if ok != c.ok || kind != c.kind || (ok && string(payload) != c.payload) {
			t.Errorf("%s/%s = %s %s %v, want %s %s %v", c.name, c.data, kind, payload, ok, c.kind, c.payload, c.ok)
		}
	}
}
