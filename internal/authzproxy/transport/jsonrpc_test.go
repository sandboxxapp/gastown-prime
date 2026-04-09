package transport

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestStream_ReadWriteMessage(t *testing.T) {
	var buf bytes.Buffer
	s := NewStream(strings.NewReader("{\"jsonrpc\":\"2.0\"}\n"), &buf)

	msg, err := s.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if string(msg) != `{"jsonrpc":"2.0"}` {
		t.Errorf("msg = %q", string(msg))
	}

	req := &Request{JSONRPC: "2.0", Method: "test"}
	if err := s.WriteMessage(req); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	if !strings.Contains(buf.String(), `"method":"test"`) {
		t.Errorf("output = %q", buf.String())
	}
}

func TestStream_WriteResponse(t *testing.T) {
	var buf bytes.Buffer
	s := NewStream(strings.NewReader(""), &buf)

	id := json.RawMessage(`1`)
	if err := s.WriteResponse(id, map[string]string{"status": "ok"}); err != nil {
		t.Fatalf("WriteResponse: %v", err)
	}

	var resp Response
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q", resp.JSONRPC)
	}
	if resp.Error != nil {
		t.Error("unexpected error in response")
	}
}

func TestStream_WriteError(t *testing.T) {
	var buf bytes.Buffer
	s := NewStream(strings.NewReader(""), &buf)

	id := json.RawMessage(`42`)
	if err := s.WriteError(id, -32600, "Invalid Request"); err != nil {
		t.Fatalf("WriteError: %v", err)
	}

	var resp Response
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != -32600 {
		t.Errorf("code = %d", resp.Error.Code)
	}
	if resp.Error.Message != "Invalid Request" {
		t.Errorf("message = %q", resp.Error.Message)
	}
}

func TestStream_SkipsBlankLines(t *testing.T) {
	input := "\n\n{\"test\":true}\n\n"
	s := NewStream(strings.NewReader(input), &bytes.Buffer{})

	msg, err := s.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if string(msg) != `{"test":true}` {
		t.Errorf("msg = %q", string(msg))
	}
}
