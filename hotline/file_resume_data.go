package hotline

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// FileResumeData is sent when a client or server would like to resume a transfer from an offset
type FileResumeData struct {
	Format       [4]byte  // "RFLT"
	Version      [2]byte  // Always 1
	RSVD         [34]byte // Present in the Hotline protocol docs, but unused.  Left here for documentation purposes.
	ForkCount    [2]byte  // Length of ForkInfoList.  Either 2 or 3 depending on whether file has a resource fork
	ForkInfoList []ForkInfoList
}

// ForkType represents a 4-byte fork type identifier
type ForkType [4]byte

// String returns a string representation of the fork type
func (ft ForkType) String() string {
	return fmt.Sprintf("%c%c%c%c", ft[0], ft[1], ft[2], ft[3])
}

type ForkInfoList struct {
	Fork     ForkType // "DATA" or "MACR"
	DataSize [4]byte  // offset from which to resume the transfer of data
	RSVDA    [4]byte  // Present in the Hotline protocol docs, but unused.  Left here for documentation purposes.
	RSVDB    [4]byte  // Present in the Hotline protocol docs, but unused.  Left here for documentation purposes.
}

var (
	ForkTypeDATA = ForkType{0x44, 0x41, 0x54, 0x41} // DATA: Data fork
	ForkTypeMACR = ForkType{0x4d, 0x41, 0x43, 0x52} // MACR: Mac resource fork
)

func NewForkInfoList(b []byte) *ForkInfoList {
	return &ForkInfoList{
		Fork:     ForkTypeDATA,
		DataSize: [4]byte{b[0], b[1], b[2], b[3]},
		RSVDA:    [4]byte{},
		RSVDB:    [4]byte{},
	}
}

var (
	FormatRFLT = [4]byte{0x52, 0x46, 0x4C, 0x54} // File resume format: "RFLT" (?)
)

func NewFileResumeData(list []ForkInfoList) *FileResumeData {
	return &FileResumeData{
		Format:       FormatRFLT,
		Version:      [2]byte{0, 1},
		RSVD:         [34]byte{},
		ForkCount:    [2]byte{0, uint8(len(list))},
		ForkInfoList: list,
	}
}

func (frd *FileResumeData) BinaryMarshal() ([]byte, error) {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, frd.Format)
	_ = binary.Write(&buf, binary.LittleEndian, frd.Version)
	_ = binary.Write(&buf, binary.LittleEndian, frd.RSVD)
	_ = binary.Write(&buf, binary.LittleEndian, frd.ForkCount)
	for _, fil := range frd.ForkInfoList {
		_ = binary.Write(&buf, binary.LittleEndian, fil)
	}

	return buf.Bytes(), nil
}

func (frd *FileResumeData) UnmarshalBinary(b []byte) error {
	frd.Format = [4]byte{b[0], b[1], b[2], b[3]}
	frd.Version = [2]byte{b[4], b[5]}
	frd.ForkCount = [2]byte{b[40], b[41]}

	for i := 0; i < int(frd.ForkCount[1]); i++ {
		var fil ForkInfoList
		start := 42 + i*16
		end := start + 16

		r := bytes.NewReader(b[start:end])
		if err := binary.Read(r, binary.BigEndian, &fil); err != nil {
			return err
		}

		frd.ForkInfoList = append(frd.ForkInfoList, fil)
	}

	return nil
}
