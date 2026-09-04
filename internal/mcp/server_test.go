package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func drive(t *testing.T, s *Server, lines ...string) []Response {
	t.Helper()
	var out bytes.Buffer
	if err := s.Serve(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatalf("serve: %v", err)
	}
	var got []Response
	dec := json.NewDecoder(&out)
	for dec.More() {
		var r Response
		if err := dec.Decode(&r); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		got = append(got, r)
	}
	return got
}

func TestInitialize(t *testing.T) {
	s := New("tunnels-mcp", "0.1.0")
	got := drive(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	if len(got) != 1 {
		t.Fatalf("responses=%d want=1", len(got))
	}
	b, _ := json.Marshal(got[0].Result)
	var res InitializeResult
	if err := json.Unmarshal(b, &res); err != nil {
		t.Fatal(err)
	}
	if res.ProtocolVersion != ProtocolVersion {
		t.Fatalf("protocolVersion=%q want=%q", res.ProtocolVersion, ProtocolVersion)
	}
	if res.Capabilities.Tools == nil {
		t.Fatal("capabilities.tools nil")
	}
	if res.ServerInfo.Name == "" || res.ServerInfo.Version == "" {
		t.Fatal("serverInfo incomplete")
	}
}

func TestIDIsEchoedVerbatim(t *testing.T) {
	s := New("x", "1")
	for _, id := range []string{`1`, `"abc"`, `"3"`} {
		got := drive(t, s, `{"jsonrpc":"2.0","id":`+id+`,"method":"ping"}`)
		if len(got) != 1 {
			t.Fatalf("id=%s responses=%d", id, len(got))
		}
		if string(got[0].ID) != id {
			t.Fatalf("id=%s echoed=%s", id, got[0].ID)
		}
	}
}

func TestNotificationGetsNoResponse(t *testing.T) {
	s := New("x", "1")
	got := drive(t, s, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if len(got) != 0 {
		t.Fatalf("responses=%d want=0", len(got))
	}
}

func TestToolsListReflectsRegistration(t *testing.T) {
	s := New("x", "1")
	s.Register(Registration{
		Tool:    Tool{Name: "only_one", Description: "d", InputSchema: EmptySchema()},
		Handler: func(context.Context, json.RawMessage) *CallToolResult { return Text("ok") },
	})
	got := drive(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	b, _ := json.Marshal(got[0].Result)
	var res ListToolsResult
	_ = json.Unmarshal(b, &res)
	if len(res.Tools) != 1 || res.Tools[0].Name != "only_one" {
		t.Fatalf("tools=%+v", res.Tools)
	}
}

func TestEmptyToolListIsAnArray(t *testing.T) {
	s := New("x", "1")
	got := drive(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	b, _ := json.Marshal(got[0].Result)
	if !strings.Contains(string(b), `"tools":[]`) {
		t.Fatalf("want tools:[] got %s", b)
	}
}

func TestToolFailureIsAResultNotAnRPCError(t *testing.T) {
	s := New("x", "1")
	s.Register(Registration{
		Tool:    Tool{Name: "boom", Description: "d", InputSchema: EmptySchema()},
		Handler: func(context.Context, json.RawMessage) *CallToolResult { return Errorf("it failed because X") },
	})
	got := drive(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"boom"}}`)
	if got[0].Error != nil {
		t.Fatalf("rpc error=%+v", got[0].Error)
	}
	b, _ := json.Marshal(got[0].Result)
	if !strings.Contains(string(b), `"isError":true`) || !strings.Contains(string(b), "it failed because X") {
		t.Fatalf("result=%s", b)
	}
}

func TestUnknownToolIsAnRPCError(t *testing.T) {
	s := New("x", "1")
	got := drive(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nope"}}`)
	if got[0].Error == nil || got[0].Error.Code != CodeMethodNotFound {
		t.Fatalf("error=%+v", got[0].Error)
	}
}

func TestPanickingToolDoesNotKillTheServer(t *testing.T) {
	s := New("x", "1")
	s.Register(Registration{
		Tool:    Tool{Name: "panics", Description: "d", InputSchema: EmptySchema()},
		Handler: func(context.Context, json.RawMessage) *CallToolResult { panic("boom") },
	})
	got := drive(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"panics"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`)
	if len(got) != 2 {
		t.Fatalf("responses=%d want=2", len(got))
	}
	b, _ := json.Marshal(got[0].Result)
	if !strings.Contains(string(b), `"isError":true`) {
		t.Fatalf("result=%s", b)
	}
}

func TestMalformedLineGetsAParseError(t *testing.T) {
	s := New("x", "1")
	got := drive(t, s, `{not json`)
	if len(got) != 1 || got[0].Error == nil || got[0].Error.Code != CodeParse {
		t.Fatalf("got=%+v", got)
	}
	if string(got[0].ID) != "null" {
		t.Fatalf("id=%s want=null", got[0].ID)
	}
}

func TestEmptySchemaIsAnObjectWithProperties(t *testing.T) {
	b, _ := json.Marshal(EmptySchema())
	s := string(b)
	if !strings.Contains(s, `"type":"object"`) || !strings.Contains(s, `"properties":{}`) {
		t.Fatalf("schema=%s", s)
	}
}
