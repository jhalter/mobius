package hotline

import (
	"bytes"
	"encoding/binary"
)

type FileNameWithInfo struct {
	fileNameWithInfoHeader
	name []byte // File name
}

// fileNameWithInfoHeader contains the fixed length fields of FileNameWithInfo
type fileNameWithInfoHeader struct {
	Type       [4]byte // file type code
	Creator    [4]byte // File creator code
	FileSize   [4]byte // File Size in bytes
	RSVD       [4]byte
	NameScript [2]byte // ??
	NameSize   [2]byte // Length of name field
}

func (f *fileNameWithInfoHeader) nameLen() int {
	return int(binary.BigEndian.Uint16(f.NameSize[:]))
}

func (f *FileNameWithInfo) MarshalBinary() (data []byte, err error) {
	var buf bytes.Buffer
	err = binary.Write(&buf, binary.LittleEndian, f.fileNameWithInfoHeader)
	if err != nil {
		return data, err
	}

	_, err = buf.Write(f.name)
	if err != nil {
		return data, err
	}

	return buf.Bytes(), err
}

func (f *FileNameWithInfo) UnmarshalBinary(data []byte) error {
	err := binary.Read(bytes.NewReader(data), binary.BigEndian, &f.fileNameWithInfoHeader)
	if err != nil {
		return err
	}
	headerLen := binary.Size(f.fileNameWithInfoHeader)
	f.name = data[headerLen : headerLen+f.nameLen()]

	return err
}

