package hotline

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Hotline handshake process
//
// After establishing TCP connection, both client and server start the handshake process
// in order to confirm that each of them comply with requirements of the other.
// The information provided in this initial data exchange identifies protocols,
// and their versions, used in the communication. In the case where, after inspection,
// the capabilities of one of the subjects do not comply with the requirements of the other,
// the connection is dropped.
//
// The following information is sent to the server:
// Description		Size 	Data	Note
// Protocol Type		4		TRTP	0x54525450
// Sub-protocol Type	4		HOTL	User defined
// VERSION			2		1		Currently 1
// Sub-version		2		2		User defined
//
// The server replies with the following:
// Description		Size 	Data	Note
// Protocol Type		4		TRTP
// Error code		4				Error code returned by the server (0 = no error)

type handshake struct {
	Protocol    [4]byte // Must be 0x54525450 TRTP
	SubProtocol [4]byte // Must be 0x484F544C HOTL
	Version     [2]byte // Always 1 (?)
	SubVersion  [2]byte // Always 2 (?)
}

// Write implements the io.Writer interface for handshake.
func (h *handshake) Write(p []byte) (n int, err error) {
	if len(p) != handshakeSize {
		return 0, errors.New("invalid handshake size")
	}

	_ = binary.Read(bytes.NewBuffer(p), binary.BigEndian, h)

	return len(p), nil
}

// Valid checks if the handshake contains valid protocol and sub-protocol IDs.
func (h *handshake) Valid() bool {
	return h.Protocol == trtp && h.SubProtocol == hotl
}

var (
	// trtp represents the Protocol Type "TRTP" in hex
	trtp = [4]byte{0x54, 0x52, 0x54, 0x50}

	// hotl represents the Sub-protocol Type "HOTL" in hex
	hotl = [4]byte{0x48, 0x4F, 0x54, 0x4C}

	// handshakeResponse represents the server's response after a successful handshake
	// Response with "TRTP" and no error code
	handshakeResponse = [8]byte{0x54, 0x52, 0x54, 0x50, 0x00, 0x00, 0x00, 0x00}
)

const handshakeSize = 12

// performHandshake performs the handshake process.
func performHandshake(rw io.ReadWriter) error {
	var h handshake

	// Copy exactly handshakeSize bytes from rw to handshake
	if _, err := io.CopyN(&h, rw, handshakeSize); err != nil {
		return fmt.Errorf("read handshake: %w", err)
	}
	if !h.Valid() {
		return errors.New("invalid protocol or sub-protocol in handshake")
	}

	if _, err := rw.Write(handshakeResponse[:]); err != nil {
		return fmt.Errorf("send handshake response: %w", err)
	}

	return nil
}
