package hotline

import (
	"bytes"
	"encoding/binary"
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
