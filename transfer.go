package hotline

import (
	"bytes"
	"encoding/binary"
	"errors"
)

type Transfer struct {
	Protocol        [4]byte // "HTXF" 0x48545846
	ReferenceNumber [4]byte // Unique ID generated for the transfer
	DataSize        [4]byte // File size
	RSVD            [4]byte // Not implemented in Hotline Protocol
}

func NewReadTransfer(b []byte) (Transfer, error) {
	r := bytes.NewReader(b)
	var decodedEvent Transfer

	if err := binary.Read(r, binary.BigEndian, &decodedEvent); err != nil {
		return decodedEvent, err
	}

	if decodedEvent.Protocol != [4]byte{0x48, 0x54, 0x58, 0x46} {
		return decodedEvent, errors.New("invalid protocol")
	}

	return decodedEvent, nil
}

type FolderTransfer struct {
	Protocol        [4]byte // "HTXF" 0x48545846
	ReferenceNumber [4]byte // Unique ID generated for the transfer
	DataSize        [4]byte // File size
	RSVD            [4]byte // Not implemented in Hotline Protocol
	Action          [2]byte // Next file action
}

func ReadFolderTransfer(b []byte) (FolderTransfer, error) {
	r := bytes.NewReader(b)
	var decodedEvent FolderTransfer

	if err := binary.Read(r, binary.BigEndian, &decodedEvent); err != nil {
		return decodedEvent, err
	}

	return decodedEvent, nil
}
