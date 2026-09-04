package cli

import (
	"strings"
	"testing"
)

func TestTunnelOpenedCarriesTheURL(t *testing.T) {
	line := `{"seq":4,"ts":"2025-01-01T00:00:00Z","type":"tunnel_opened","public_url":"https://abc.tunnels.io","local_addr":"127.0.0.1:3000","protocol":"http"}`
	ev, ok := ParseEvent([]byte(line))
	if !ok {
		t.Fatal("parse failed")
	}
	if ev.Type != EvTunnelOpened {
		t.Fatalf("type=%q", ev.Type)
	}
	if ev.PublicURL != "https://abc.tunnels.io" {
		t.Fatalf("public_url=%q", ev.PublicURL)
	}
	if ev.LocalAddr != "127.0.0.1:3000" || ev.Protocol != "http" {
		t.Fatalf("local_addr=%q protocol=%q", ev.LocalAddr, ev.Protocol)
	}
}

func TestDroppedCarriesTheCount(t *testing.T) {
	ev, ok := ParseEvent([]byte(`{"seq":9,"ts":"t","type":"dropped","count":17}`))
	if !ok || ev.Type != EvDropped {
		t.Fatal("parse failed")
	}
	if ev.Count != 17 {
		t.Fatalf("count=%d want=17", ev.Count)
	}
}

func TestFailureEventsKeepTheirReason(t *testing.T) {
	cases := []struct{ line, typ, want string }{
		{`{"seq":1,"ts":"t","type":"error","message":"bad token"}`, EvError, "bad token"},
		{`{"seq":2,"ts":"t","type":"disconnected","error":"connection refused"}`, EvDisconnected, "connection refused"},
		{`{"seq":3,"ts":"t","type":"exiting","reason":"signal"}`, EvExiting, "signal"},
	}
	for _, c := range cases {
		ev, ok := ParseEvent([]byte(c.line))
		if !ok || ev.Type != c.typ {
			t.Fatalf("%s: got %+v", c.typ, ev)
		}
		got := ev.Message + ev.ErrorText + ev.Reason
		if !strings.Contains(got, c.want) {
			t.Fatalf("%s: reason=%q", c.typ, got)
		}
	}
}

func TestUnrecognisedLinesAreSkipped(t *testing.T) {
	for _, line := range []string{"", "not json", `{"no":"type"}`, `[]`} {
		if _, ok := ParseEvent([]byte(line)); ok {
			t.Fatalf("parsed %q", line)
		}
	}
}

func TestUnknownEventTypeStillParses(t *testing.T) {
	ev, ok := ParseEvent([]byte(`{"seq":1,"ts":"t","type":"some_future_event","x":1}`))
	if !ok {
		t.Fatal("parse failed")
	}
	if ev.Type != "some_future_event" {
		t.Fatalf("type=%q", ev.Type)
	}
}

func TestAvailableNamesTheFix(t *testing.T) {
	err := NewRunner("definitely-not-a-real-binary-xyz", "tnl_x").Available()
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "PATH") || !strings.Contains(err.Error(), "TUNNELS_CLI") {
		t.Fatalf("msg=%q", err.Error())
	}
}

func TestDefaultBinaryName(t *testing.T) {
	if got := NewRunner("", "tnl_x").Bin; got != "tunnels" {
		t.Fatalf("bin=%q want=tunnels", got)
	}
}
