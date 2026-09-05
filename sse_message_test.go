package hyperserve_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/osauer/hyperserve/v2"
)

func TestSSEMessageWireFormat(t *testing.T) {
	tests := []struct {
		name string
		msg  *hyperserve.SSEMessage
		want string
	}{
		{
			name: "existing constructor",
			msg:  hyperserve.NewSSEMessage("ready"),
			want: "event: message\ndata: ready\n\n",
		},
		{
			name: "existing keyed literal",
			msg:  &hyperserve.SSEMessage{Event: "status", Data: "ready"},
			want: "event: status\ndata: ready\n\n",
		},
		{
			name: "zero value",
			msg:  &hyperserve.SSEMessage{},
			want: "data: null\n\n",
		},
		{
			name: "event ID with structured data",
			msg:  &hyperserve.SSEMessage{Event: "status", Data: map[string]int{"count": 2}, ID: "42"},
			want: "event: status\nid: 42\ndata: {\"count\":2}\n\n",
		},
		{
			name: "ID without event type",
			msg:  &hyperserve.SSEMessage{Data: "ready", ID: "0"},
			want: "id: 0\ndata: ready\n\n",
		},
		{
			name: "empty ID does not reset",
			msg:  &hyperserve.SSEMessage{Data: "ready", ID: ""},
			want: "data: ready\n\n",
		},
		{
			name: "preserve whitespace colon and Unicode",
			msg:  &hyperserve.SSEMessage{Data: "ready", ID: " \t進捗: 42 "},
			want: "id:  \t進捗: 42 \ndata: ready\n\n",
		},
		{
			name: "ID with multiline data",
			msg:  &hyperserve.SSEMessage{Data: []byte("first\r\nid: payload\rlast\n"), ID: "42"},
			want: "id: 42\ndata: first\ndata: id: payload\ndata: last\ndata: \n\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.msg.String(); got != tt.want {
				t.Fatalf("wire message = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSSEMessageOmitsInvalidID(t *testing.T) {
	for name, id := range map[string]string{
		"LF field injection":   "42\nid: forged",
		"CR field injection":   "42\rdata: forged",
		"CRLF event injection": "42\r\n\r\nevent: forged\r\ndata: forged",
		"leading line break":   "\nid: forged",
		"trailing line break":  "42\n",
		"embedded NUL":         "4\x002",
		"NUL only":             "\x00",
		"invalid UTF-8":        "42\xff",
		"truncated UTF-8":      "42\xe2\x82",
	} {
		t.Run(name, func(t *testing.T) {
			msg := hyperserve.SSEMessage{Event: "status", Data: "ready", ID: id}
			const want = "event: status\ndata: ready\n\n"
			if got := msg.String(); got != want {
				t.Fatalf("wire message with invalid ID = %q, want %q", got, want)
			}
		})
	}
}

func TestSSEMessageJSON(t *testing.T) {
	for _, tt := range []struct {
		id   string
		want string
	}{
		{"", `{"event":"message","data":"ready"}`},
		{"42", `{"event":"message","data":"ready","id":"42"}`},
	} {
		msg := hyperserve.NewSSEMessage("ready")
		msg.ID = tt.id
		encoded, err := json.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != tt.want {
			t.Fatalf("JSON = %s, want %s", encoded, tt.want)
		}
		var decoded hyperserve.SSEMessage
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.String() != msg.String() {
			t.Fatalf("JSON round trip changed the wire message: %q", decoded.String())
		}
	}
}

func ExampleSSEMessage() {
	msg := hyperserve.NewSSEMessage("ready")
	msg.Event = "status"
	msg.ID = "42"
	fmt.Print(msg)
	// Output:
	// event: status
	// id: 42
	// data: ready
}
