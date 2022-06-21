package hotline

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

type handshake struct {
	Protocol    [4]byte // Must be 0x54525450 TRTP
	SubProtocol [4]byte
	Version     [2]byte // Always 1
	SubVersion  [2]byte
}

var trtp = [4]byte{0x54, 0x52, 0x54, 0x50}

// Handshake
// After establishing TCP connection, both client and server start the handshake process
// in order to confirm that each of them comply with requirements of the other.
// The information provided in this initial data exchange identifies protocols,
// and their versions, used in the communication. In the case where, after inspection,
// the capabilities of one of the subjects do not comply with the requirements of the other,
// the connection is dropped.
//
// The following information is sent to the server:
// Description		Size 	Data	Note
// Protocol ID		4		TRTP	0x54525450
// Sub-protocol ID	4		HOTL	User defined
// VERSION			2		1		Currently 1
// Sub-version		2		2		User defined
//
// The server replies with the following:
// Description		Size 	Data	Note
// Protocol ID		4		TRTP
// Error code		4				Error code returned by the server (0 = no error)
func Handshake(rw io.ReadWriter) error {
	handshakeBuf := make([]byte, 12)
	if _, err := io.ReadFull(rw, handshakeBuf); err != nil {
		return err
	}

	var h handshake
	r := bytes.NewReader(handshakeBuf)
	if err := binary.Read(r, binary.BigEndian, &h); err != nil {
		return err
	}

	if h.Protocol != trtp {
		return errors.New("invalid handshake")
	}

	_, err := rw.Write([]byte{84, 82, 84, 80, 0, 0, 0, 0})
	return err
}
