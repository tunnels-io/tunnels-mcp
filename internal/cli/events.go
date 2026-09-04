package cli

import "encoding/json"

type Event struct {
	Seq  uint64 `json:"seq"`
	TS   string `json:"ts"`
	Type string `json:"type"`

	PublicURL string `json:"public_url,omitempty"`
	LocalAddr string `json:"local_addr,omitempty"`
	Protocol  string `json:"protocol,omitempty"`

	By         string `json:"by,omitempty"`
	ReasonCode string `json:"reason_code,omitempty"`
	Reason     string `json:"reason,omitempty"`

	ErrorText string `json:"error,omitempty"`

	Message string `json:"message,omitempty"`

	DelayMS uint64 `json:"delay_ms,omitempty"`

	Count uint64 `json:"count,omitempty"`

	SchemaVersion uint32 `json:"schema_version,omitempty"`
	ClientVersion string `json:"client_version,omitempty"`
}

const (
	EvHello         = "hello"
	EvConnecting    = "connecting"
	EvAuthenticated = "authenticated"
	EvTunnelOpened  = "tunnel_opened"
	EvTunnelClosed  = "tunnel_closed"
	EvDisconnected  = "disconnected"
	EvReconnecting  = "reconnecting"
	EvError         = "error"
	EvDropped       = "dropped"
	EvExiting       = "exiting"
)

func ParseEvent(line []byte) (Event, bool) {
	var e Event
	if err := json.Unmarshal(line, &e); err != nil || e.Type == "" {
		return Event{}, false
	}
	return e, true
}
