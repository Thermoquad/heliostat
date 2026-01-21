// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (c) 2025 Kaz Walker, Thermoquad

package cmd

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"go.bug.st/serial"
	"golang.org/x/term"
)

// Connection provides a common interface for reading/writing bytes from serial or WebSocket
type Connection interface {
	io.Reader
	io.Writer
	io.Closer
}

// ByteReader is an alias for backward compatibility
type ByteReader = Connection

// SerialConnection wraps a serial port
type SerialConnection struct {
	port serial.Port
}

func (s *SerialConnection) Read(p []byte) (int, error) {
	return s.port.Read(p)
}

func (s *SerialConnection) Write(p []byte) (int, error) {
	return s.port.Write(p)
}

func (s *SerialConnection) Close() error {
	return s.port.Close()
}

// ErrConnectionClosed is returned when reading from a closed WebSocket connection
var ErrConnectionClosed = fmt.Errorf("websocket connection closed")

// WebSocketConnection wraps a WebSocket connection for byte-level reading
type WebSocketConnection struct {
	conn      *websocket.Conn
	buf       []byte
	bufOffset int
	closed    bool // Track if connection has failed/closed
}

func (w *WebSocketConnection) Read(p []byte) (int, error) {
	// Return immediately if connection is known to be closed
	if w.closed {
		return 0, ErrConnectionClosed
	}

	// If we have buffered data, return it first
	if w.bufOffset < len(w.buf) {
		n := copy(p, w.buf[w.bufOffset:])
		w.bufOffset += n
		return n, nil
	}

	// Read next message from WebSocket (non-recursive loop to avoid stack overflow)
	for {
		messageType, data, err := w.conn.ReadMessage()
		if err != nil {
			// Mark connection as closed to prevent further read attempts
			w.closed = true
			return 0, err
		}

		// We only handle binary messages for Fusain protocol
		if messageType != websocket.BinaryMessage {
			// Skip non-binary messages and continue loop
			continue
		}

		// Buffer the message and return what fits
		w.buf = data
		w.bufOffset = 0
		n := copy(p, w.buf)
		w.bufOffset = n
		return n, nil
	}
}

func (w *WebSocketConnection) Write(p []byte) (int, error) {
	err := w.conn.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *WebSocketConnection) Close() error {
	return w.conn.Close()
}

// OpenSerialConnection opens a serial port connection
func OpenSerialConnection(portName string, baudRate int) (ByteReader, error) {
	mode := &serial.Mode{
		BaudRate: baudRate,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(portName, mode)
	if err != nil {
		return nil, fmt.Errorf("failed to open serial port %s: %v", portName, err)
	}

	return &SerialConnection{port: port}, nil
}

// OpenWebSocketConnection opens a WebSocket connection with HTTP Basic auth
func OpenWebSocketConnection(wsURL, username, password string, skipSSLVerify bool) (ByteReader, error) {
	// Parse and validate URL
	u, err := url.Parse(wsURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}

	// Validate scheme
	switch u.Scheme {
	case "ws", "wss":
		// OK
	default:
		return nil, fmt.Errorf("unsupported URL scheme: %s (use ws:// or wss://)", u.Scheme)
	}

	// Create dialer with timeout
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	// Configure TLS for wss://
	if u.Scheme == "wss" {
		dialer.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: skipSSLVerify,
		}
	}

	// Build HTTP headers with Basic auth
	headers := http.Header{}
	if username != "" && password != "" {
		credentials := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		headers.Set("Authorization", "Basic "+credentials)
	}

	// Connect
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, resp, err := dialer.DialContext(ctx, wsURL, headers)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("WebSocket connection failed (HTTP %d): %v", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("WebSocket connection failed: %v", err)
	}

	return &WebSocketConnection{conn: conn}, nil
}

// GetPassword retrieves password from environment or prompts user
func GetPassword() (string, error) {
	// First check environment variable
	if pw := os.Getenv("FUSAIN_PASSWORD"); pw != "" {
		return pw, nil
	}

	// Prompt user for password (hide input)
	fmt.Fprint(os.Stderr, "Password: ")

	// Read password without echo
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		// Fallback to regular input if terminal functions fail
		reader := bufio.NewReader(os.Stdin)
		password, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read password: %v", err)
		}
		fmt.Fprintln(os.Stderr) // newline after password
		return strings.TrimSpace(password), nil
	}

	fmt.Fprintln(os.Stderr) // newline after password
	return string(passwordBytes), nil
}

// OpenConnection opens either a serial or WebSocket connection based on flags
func OpenConnection() (ByteReader, string, error) {
	if wsURL != "" {
		// WebSocket mode
		password := ""
		if wsUsername != "" {
			var err error
			password, err = GetPassword()
			if err != nil {
				return nil, "", err
			}
		}

		conn, err := OpenWebSocketConnection(wsURL, wsUsername, password, wsNoSSLVerify)
		if err != nil {
			return nil, "", err
		}

		return conn, fmt.Sprintf("WebSocket: %s", wsURL), nil
	}

	if portName != "" {
		// Serial mode
		conn, err := OpenSerialConnection(portName, baudRate)
		if err != nil {
			return nil, "", err
		}

		return conn, fmt.Sprintf("Serial: %s @ %d baud", portName, baudRate), nil
	}

	return nil, "", fmt.Errorf("either --port or --url must be specified")
}
