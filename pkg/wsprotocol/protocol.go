// Package wsprotocol defines message types and constants used for WebSocket
// communication between the Hub and Runtime Hosts.
package wsprotocol

import (
	"encoding/json"
	"time"
)

// Control channel message types (Hub ↔ Runtime Host)
const (
	// TypeConnect is sent by Runtime Host to initiate connection
	TypeConnect = "connect"
	// TypeConnected is sent by Hub to confirm connection
	TypeConnected = "connected"
	// TypeRequest is sent by Hub to tunnel an HTTP request
	TypeRequest = "request"
	// TypeResponse is sent by Runtime Host with HTTP response
	TypeResponse = "response"
	// TypeStream is sent for streaming data (e.g., PTY)
	TypeStream = "stream"
	// TypeStreamOpen is sent to open a new stream
	TypeStreamOpen = "stream_open"
	// TypeStreamClose is sent to close a stream
	TypeStreamClose = "stream_close"
	// TypeEvent is sent for async events (heartbeat, status updates)
	TypeEvent = "event"
	// TypePing is sent for keepalive
	TypePing = "ping"
	// TypePong is the response to ping
	TypePong = "pong"
	// TypeError is sent when an error occurs
	TypeError = "error"
)

// PTY message types (Client ↔ Hub)
const (
	// TypeData contains terminal data
	TypeData = "data"
	// TypeResize contains terminal resize info
	TypeResize = "resize"
)

// Event types for TypeEvent messages
const (
	EventHeartbeat    = "heartbeat"
	EventAgentStatus  = "agent_status"
	EventAgentOutput  = "agent_output"
	EventStreamReady  = "stream_ready"
	EventStreamClosed = "stream_closed"
)

// Stream types for TypeStreamOpen
const (
	StreamTypePTY    = "pty"
	StreamTypeEvents = "events"
	StreamTypeLogs   = "logs"
)

// Envelope is the base message structure for all WebSocket messages.
// All messages must have a Type field.
type Envelope struct {
	Type string `json:"type"`
}

// ConnectMessage is sent by Runtime Host when establishing control channel.
type ConnectMessage struct {
	Type      string   `json:"type"` // Always "connect"
	BrokerID string   `json:"brokerId"`
	Version   string   `json:"version"`
	Groves    []string `json:"groves,omitempty"`    // Grove IDs this host serves
	Timestamp int64    `json:"timestamp,omitempty"` // Unix timestamp
}

// ConnectedMessage is sent by Hub to confirm successful connection.
type ConnectedMessage struct {
	Type           string `json:"type"` // Always "connected"
	BrokerID string `json:"brokerId"`
	SessionID      string `json:"sessionId"`      // Unique session identifier
	PingIntervalMs int    `json:"pingIntervalMs"` // Expected ping interval
}

// RequestEnvelope tunnels an HTTP request through the control channel.
type RequestEnvelope struct {
	Type      string            `json:"type"` // Always "request"
	RequestID string            `json:"requestId"`
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Query     string            `json:"query,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      []byte            `json:"body,omitempty"` // Base64 encoded in JSON
}

// ResponseEnvelope carries an HTTP response back through the control channel.
type ResponseEnvelope struct {
	Type       string            `json:"type"` // Always "response"
	RequestID  string            `json:"requestId"`
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       []byte            `json:"body,omitempty"` // Base64 encoded in JSON
}

// StreamOpenMessage requests opening a new multiplexed stream.
type StreamOpenMessage struct {
	Type       string `json:"type"` // Always "stream_open"
	StreamID   string `json:"streamId"`
	StreamType string `json:"streamType"` // "pty", "events", "logs"
	AgentID    string `json:"agentId,omitempty"`
	Cols       int    `json:"cols,omitempty"` // For PTY streams
	Rows       int    `json:"rows,omitempty"` // For PTY streams
}

// StreamFrame carries data for a multiplexed stream.
type StreamFrame struct {
	Type     string `json:"type"` // Always "stream"
	StreamID string `json:"streamId"`
	Data     []byte `json:"data,omitempty"` // Base64 encoded in JSON
}

// StreamCloseMessage signals stream termination.
type StreamCloseMessage struct {
	Type     string `json:"type"` // Always "stream_close"
	StreamID string `json:"streamId"`
	Reason   string `json:"reason,omitempty"`
	Code     int    `json:"code,omitempty"` // Optional exit code
}

// EventMessage carries async events.
type EventMessage struct {
	Type    string          `json:"type"` // Always "event"
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// HeartbeatPayload is the payload for heartbeat events.
type HeartbeatPayload struct {
	Timestamp    int64            `json:"timestamp"`
	ActiveAgents int              `json:"activeAgents"`
	CPUPercent   float64          `json:"cpuPercent,omitempty"`
	MemoryMB     int64            `json:"memoryMb,omitempty"`
	AgentStates  map[string]State `json:"agentStates,omitempty"`
}

// State represents agent state in heartbeat.
type State struct {
	Status    string `json:"status"`
	UpdatedAt int64  `json:"updatedAt"`
}

// PingMessage is a keepalive ping.
type PingMessage struct {
	Type      string `json:"type"` // Always "ping"
	Timestamp int64  `json:"timestamp"`
}

// PongMessage is the response to ping.
type PongMessage struct {
	Type      string `json:"type"` // Always "pong"
	Timestamp int64  `json:"timestamp"`
}

// ErrorMessage indicates an error occurred.
type ErrorMessage struct {
	Type      string `json:"type"` // Always "error"
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty"` // If related to a request
	StreamID  string `json:"streamId,omitempty"`  // If related to a stream
}

// PTY-specific messages for client connections

// PTYDataMessage carries terminal I/O data.
type PTYDataMessage struct {
	Type string `json:"type"` // Always "data"
	Data []byte `json:"data"` // Base64 encoded in JSON
}

// PTYResizeMessage carries terminal resize events.
type PTYResizeMessage struct {
	Type string `json:"type"` // Always "resize"
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

// Common error codes
const (
	ErrCodeInvalidMessage   = "invalid_message"
	ErrCodeAuthFailed       = "auth_failed"
	ErrCodeHostNotFound     = "host_not_found"
	ErrCodeAgentNotFound    = "agent_not_found"
	ErrCodeStreamNotFound   = "stream_not_found"
	ErrCodeStreamFailed     = "stream_failed"
	ErrCodeTimeout          = "timeout"
	ErrCodeHostDisconnected = "host_disconnected"
	ErrCodeInternalError    = "internal_error"
)

// ParseEnvelope extracts the message type from a raw JSON message.
func ParseEnvelope(data []byte) (*Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	return &env, nil
}

// NewConnectMessage creates a connect message for a runtime host.
func NewConnectMessage(brokerID, version string, groves []string) *ConnectMessage {
	return &ConnectMessage{
		Type:      TypeConnect,
		BrokerID:    brokerID,
		Version:   version,
		Groves:    groves,
		Timestamp: time.Now().Unix(),
	}
}

// NewConnectedMessage creates a connected response.
func NewConnectedMessage(brokerID, sessionID string, pingIntervalMs int) *ConnectedMessage {
	return &ConnectedMessage{
		Type:           TypeConnected,
		BrokerID:         brokerID,
		SessionID:      sessionID,
		PingIntervalMs: pingIntervalMs,
	}
}

// NewRequestEnvelope creates a request envelope for HTTP tunneling.
func NewRequestEnvelope(requestID, method, path, query string, headers map[string]string, body []byte) *RequestEnvelope {
	return &RequestEnvelope{
		Type:      TypeRequest,
		RequestID: requestID,
		Method:    method,
		Path:      path,
		Query:     query,
		Headers:   headers,
		Body:      body,
	}
}

// NewResponseEnvelope creates a response envelope.
func NewResponseEnvelope(requestID string, statusCode int, headers map[string]string, body []byte) *ResponseEnvelope {
	return &ResponseEnvelope{
		Type:       TypeResponse,
		RequestID:  requestID,
		StatusCode: statusCode,
		Headers:    headers,
		Body:       body,
	}
}

// NewStreamOpenMessage creates a stream open request.
func NewStreamOpenMessage(streamID, streamType, agentID string, cols, rows int) *StreamOpenMessage {
	return &StreamOpenMessage{
		Type:       TypeStreamOpen,
		StreamID:   streamID,
		StreamType: streamType,
		AgentID:    agentID,
		Cols:       cols,
		Rows:       rows,
	}
}

// NewStreamFrame creates a stream data frame.
func NewStreamFrame(streamID string, data []byte) *StreamFrame {
	return &StreamFrame{
		Type:     TypeStream,
		StreamID: streamID,
		Data:     data,
	}
}

// NewStreamCloseMessage creates a stream close message.
func NewStreamCloseMessage(streamID, reason string, code int) *StreamCloseMessage {
	return &StreamCloseMessage{
		Type:     TypeStreamClose,
		StreamID: streamID,
		Reason:   reason,
		Code:     code,
	}
}

// NewErrorMessage creates an error message.
func NewErrorMessage(code, message, requestID, streamID string) *ErrorMessage {
	return &ErrorMessage{
		Type:      TypeError,
		Code:      code,
		Message:   message,
		RequestID: requestID,
		StreamID:  streamID,
	}
}

// NewPingMessage creates a ping message.
func NewPingMessage() *PingMessage {
	return &PingMessage{
		Type:      TypePing,
		Timestamp: time.Now().UnixMilli(),
	}
}

// NewPongMessage creates a pong message.
func NewPongMessage() *PongMessage {
	return &PongMessage{
		Type:      TypePong,
		Timestamp: time.Now().UnixMilli(),
	}
}

// NewPTYDataMessage creates a PTY data message.
func NewPTYDataMessage(data []byte) *PTYDataMessage {
	return &PTYDataMessage{
		Type: TypeData,
		Data: data,
	}
}

// NewPTYResizeMessage creates a PTY resize message.
func NewPTYResizeMessage(cols, rows int) *PTYResizeMessage {
	return &PTYResizeMessage{
		Type: TypeResize,
		Cols: cols,
		Rows: rows,
	}
}
