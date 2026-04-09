// Package transport implements JSON-RPC 2.0 communication over stdio and sockets.
package transport

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Request is a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error is a JSON-RPC 2.0 error object.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// ReadWriter handles reading/writing JSON-RPC messages over a stream.
type ReadWriter interface {
	ReadMessage() ([]byte, error)
	WriteMessage(msg any) error
	WriteResponse(id json.RawMessage, result any) error
	WriteError(id json.RawMessage, code int, message string) error
}

// Stream implements ReadWriter over io.Reader/io.Writer.
type Stream struct {
	reader *bufio.Reader
	writer io.Writer
	mu     sync.Mutex
}

// NewStream creates a transport over reader/writer streams.
func NewStream(r io.Reader, w io.Writer) *Stream {
	return &Stream{reader: bufio.NewReader(r), writer: w}
}

// ReadMessage reads one newline-delimited JSON message.
func (s *Stream) ReadMessage() ([]byte, error) {
	for {
		line, err := s.reader.ReadBytes('\n')
		if err != nil {
			return nil, err
		}
		for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
			line = line[:len(line)-1]
		}
		if len(line) > 0 {
			return line, nil
		}
	}
}

// WriteMessage writes a JSON message as a newline-delimited line.
func (s *Stream) WriteMessage(msg any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	_, err = s.writer.Write(append(data, '\n'))
	return err
}

// WriteResponse writes a JSON-RPC success response.
func (s *Stream) WriteResponse(id json.RawMessage, result any) error {
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return s.WriteMessage(&Response{JSONRPC: "2.0", ID: id, Result: resultBytes})
}

// WriteError writes a JSON-RPC error response.
func (s *Stream) WriteError(id json.RawMessage, code int, message string) error {
	return s.WriteMessage(&Response{JSONRPC: "2.0", ID: id, Error: &Error{Code: code, Message: message}})
}
