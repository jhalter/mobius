package hotline

import (
	"bytes"
	"testing"
)

func TestHandshakeWrite(t *testing.T) {
	tests := []struct {
		name          string
		input         []byte
		expected      handshake
		expectedError string
	}{
		{
			name:  "Valid Handshake",
			input: []byte{0x54, 0x52, 0x54, 0x50, 0x48, 0x4F, 0x54, 0x4C, 0x00, 0x01, 0x00, 0x02},
			expected: handshake{
				Protocol:    [4]byte{0x54, 0x52, 0x54, 0x50},
				SubProtocol: [4]byte{0x48, 0x4F, 0x54, 0x4C},
				Version:     [2]byte{0x00, 0x01},
				SubVersion:  [2]byte{0x00, 0x02},
			},
			expectedError: "",
		},
		{
			name:          "Invalid Handshake Size",
			input:         []byte{0x54, 0x52, 0x54, 0x50},
			expected:      handshake{},
			expectedError: "invalid handshake size",
		},
		{
			name:          "Empty Handshake Data",
			input:         []byte{},
			expected:      handshake{},
			expectedError: "invalid handshake size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h handshake
			n, err := h.Write(tt.input)

			if tt.expectedError != "" {
				if err == nil || err.Error() != tt.expectedError {
					t.Fatalf("expected error %q, got %q", tt.expectedError, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if n != handshakeSize {
					t.Fatalf("expected %d bytes written, got %d", handshakeSize, n)
				}
				if h != tt.expected {
					t.Fatalf("expected handshake %+v, got %+v", tt.expected, h)
				}
			}
		})
	}
}

func TestHandshakeValid(t *testing.T) {
	tests := []struct {
		name     string
		input    handshake
		expected bool
	}{
		{
			name: "Valid Handshake",
			input: handshake{
				Protocol:    [4]byte{0x54, 0x52, 0x54, 0x50}, // TRTP
				SubProtocol: [4]byte{0x48, 0x4F, 0x54, 0x4C}, // HOTL
				Version:     [2]byte{0x00, 0x01},
				SubVersion:  [2]byte{0x00, 0x02},
			},
			expected: true,
		},
		{
			name: "Invalid Protocol",
			input: handshake{
				Protocol:    [4]byte{0x00, 0x00, 0x00, 0x00},
				SubProtocol: [4]byte{0x48, 0x4F, 0x54, 0x4C}, // HOTL
				Version:     [2]byte{0x00, 0x01},
				SubVersion:  [2]byte{0x00, 0x02},
			},
			expected: false,
		},
		{
			name: "Invalid SubProtocol",
			input: handshake{
				Protocol:    [4]byte{0x54, 0x52, 0x54, 0x50}, // TRTP
				SubProtocol: [4]byte{0x00, 0x00, 0x00, 0x00},
				Version:     [2]byte{0x00, 0x01},
				SubVersion:  [2]byte{0x00, 0x02},
			},
			expected: false,
		},
		{
			name: "Invalid Protocol and SubProtocol",
			input: handshake{
				Protocol:    [4]byte{0x00, 0x00, 0x00, 0x00},
				SubProtocol: [4]byte{0x00, 0x00, 0x00, 0x00},
				Version:     [2]byte{0x00, 0x01},
				SubVersion:  [2]byte{0x00, 0x02},
			},
			expected: false,
		},
		{
			name: "Valid Handshake with Different Version",
			input: handshake{
				Protocol:    [4]byte{0x54, 0x52, 0x54, 0x50}, // TRTP
				SubProtocol: [4]byte{0x48, 0x4F, 0x54, 0x4C}, // HOTL
				Version:     [2]byte{0x00, 0x02},
				SubVersion:  [2]byte{0x00, 0x03},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input.Valid()
			if result != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// readWriteBuffer combines input and output buffers to implement io.ReadWriter
type readWriteBuffer struct {
	input  *bytes.Buffer
	output *bytes.Buffer
}

func (rw *readWriteBuffer) Read(p []byte) (int, error) {
	return rw.input.Read(p)
}

func (rw *readWriteBuffer) Write(p []byte) (int, error) {
	return rw.output.Write(p)
}

func TestPerformHandshake(t *testing.T) {
	tests := []struct {
		name           string
		input          []byte
		expectedOutput []byte
		expectedError  string
	}{
		{
			name: "Valid Handshake",
			input: []byte{
				0x54, 0x52, 0x54, 0x50, // TRTP
				0x48, 0x4F, 0x54, 0x4C, // HOTL
				0x00, 0x01, 0x00, 0x02, // Version 1, SubVersion 2
			},
			expectedOutput: []byte{0x54, 0x52, 0x54, 0x50, 0x00, 0x00, 0x00, 0x00},
			expectedError:  "",
		},
		{
			name: "Invalid Handshake Size",
			input: []byte{
				0x54, 0x52, 0x54, 0x50, // TRTP
			},
			expectedOutput: nil,
			expectedError:  "read handshake: invalid handshake size",
		},
		{
			name: "Invalid Protocol",
			input: []byte{
				0x00, 0x00, 0x00, 0x00, // Invalid protocol
				0x48, 0x4F, 0x54, 0x4C, // HOTL
				0x00, 0x01, 0x00, 0x02, // Version 1, SubVersion 2
			},
			expectedOutput: nil,
			expectedError:  "invalid protocol or sub-protocol in handshake",
		},
		{
			name: "Invalid SubProtocol",
			input: []byte{
				0x54, 0x52, 0x54, 0x50, // TRTP
				0x00, 0x00, 0x00, 0x00, // Invalid sub-protocol
				0x00, 0x01, 0x00, 0x02, // Version 1, SubVersion 2
			},
			expectedOutput: nil,
			expectedError:  "invalid protocol or sub-protocol in handshake",
		},
		{
			name: "Binary Read Error",
			input: []byte{
				0xFF, 0xFF, 0xFF, 0xFF, // Invalid data
				0xFF, 0xFF, 0xFF, 0xFF,
				0xFF, 0xFF, 0xFF, 0xFF,
			},
			expectedOutput: nil,
			expectedError:  "invalid protocol or sub-protocol in handshake",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputBuffer := bytes.NewBuffer(tt.input)
			outputBuffer := &bytes.Buffer{}
			rw := &readWriteBuffer{
				input:  inputBuffer,
				output: outputBuffer,
			}

			err := performHandshake(rw)

			if tt.expectedError != "" {
				if err == nil || err.Error() != tt.expectedError {
					t.Fatalf("expected error %q, got %q", tt.expectedError, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				output := outputBuffer.Bytes()
				if !bytes.Equal(output, tt.expectedOutput) {
					t.Fatalf("expected output %v, got %v", tt.expectedOutput, output)
				}
			}
		})
	}
}
