package hotline

import (
	"encoding/binary"
	"io"
)

type flattenedFileObject struct {
	FlatFileHeader                FlatFileHeader
	FlatFileInformationForkHeader FlatFileForkHeader
	FlatFileInformationFork       FlatFileInformationFork
	FlatFileDataForkHeader        FlatFileForkHeader
	FlatFileResForkHeader         FlatFileForkHeader
}

// FlatFileHeader is the first section of a "Flattened File Object".  All fields have static values.
type FlatFileHeader struct {
	Format    [4]byte  // Always "FILP"
	Version   [2]byte  // Always 1
	RSVD      [16]byte // Always empty zeros
	ForkCount [2]byte  // Number of forks, either 2 or 3 if there is a resource fork
}

type FlatFileInformationFork struct {
	Platform         []byte // Operating System used. ("AMAC" or "MWIN")
	TypeSignature    []byte // File type signature
	CreatorSignature []byte // File creator signature
	Flags            []byte
	PlatformFlags    []byte
	RSVD             []byte
	CreateDate       []byte
	ModifyDate       []byte
	NameScript       []byte
	NameSize         []byte // Length of file name (Maximum 128 characters)
	Name             []byte // File name
	CommentSize      []byte // Length of the comment
	Comment          []byte // File comment
}

func NewFlatFileInformationFork(fileName string, modifyTime []byte, typeSignature string, creatorSignature string) FlatFileInformationFork {
	return FlatFileInformationFork{
		Platform:         []byte("AMAC"),           // TODO: Remove hardcode to support "AWIN" Platform (maybe?)
		TypeSignature:    []byte(typeSignature),    // TODO: Don't infer types from filename
		CreatorSignature: []byte(creatorSignature), // TODO: Don't infer types from filename
		Flags:            []byte{0, 0, 0, 0},       // TODO: What is this?
		PlatformFlags:    []byte{0, 0, 1, 0},       // TODO: What is this?
		RSVD:             make([]byte, 32),         // Unimplemented in Hotline Protocol
		CreateDate:       modifyTime,               // some filesystems don't support createTime
		ModifyDate:       modifyTime,
		NameScript:       make([]byte, 2), // TODO: What is this?
		Name:             []byte(fileName),
		CommentSize:      []byte{0, 0},
		Comment:          []byte{}, // TODO: implement (maybe?)
	}
}

func (ffif *FlatFileInformationFork) friendlyType() []byte {
	if name, ok := friendlyCreatorNames[string(ffif.TypeSignature)]; ok {
		return []byte(name)
	}
	return ffif.TypeSignature
}

func (ffif *FlatFileInformationFork) friendlyCreator() []byte {
	if name, ok := friendlyCreatorNames[string(ffif.CreatorSignature)]; ok {
		return []byte(name)
	}
	return ffif.CreatorSignature
}

func (ffif *FlatFileInformationFork) setComment(comment []byte) error {
	ffif.CommentSize = make([]byte, 2)
	ffif.Comment = comment
	binary.BigEndian.PutUint16(ffif.CommentSize, uint16(len(comment)))

	// TODO: return err if comment is too long
	return nil
}

// DataSize calculates the size of the flat file information fork, which is
// 72 bytes for the fixed length fields plus the length of the Name + Comment
func (ffif *FlatFileInformationFork) DataSize() []byte {
	size := make([]byte, 4)

	dataSize := len(ffif.Name) + len(ffif.Comment) + 74 // 74 = len of fixed size headers

	binary.BigEndian.PutUint32(size, uint32(dataSize))

	return size
}

func (ffif *FlatFileInformationFork) Size() [4]byte {
	size := [4]byte{}

	dataSize := len(ffif.Name) + len(ffif.Comment) + 74 // 74 = len of fixed size headers

	binary.BigEndian.PutUint32(size[:], uint32(dataSize))

	return size
}

func (ffo *flattenedFileObject) TransferSize(offset int64) []byte {
	// get length of the flattenedFileObject, including the info fork
	payloadSize := len(ffo.BinaryMarshal())

	// length of data fork
	dataSize := binary.BigEndian.Uint32(ffo.FlatFileDataForkHeader.DataSize[:])

	// length of resource fork
	resForkSize := binary.BigEndian.Uint32(ffo.FlatFileResForkHeader.DataSize[:])

	size := make([]byte, 4)
	binary.BigEndian.PutUint32(size[:], dataSize+resForkSize+uint32(payloadSize)-uint32(offset))

	return size
}

func (ffif *FlatFileInformationFork) ReadNameSize() []byte {
	size := make([]byte, 2)
	binary.BigEndian.PutUint16(size, uint16(len(ffif.Name)))

	return size
}

type FlatFileForkHeader struct {
	ForkType        [4]byte // Either INFO, DATA or MACR
	CompressionType [4]byte
	RSVD            [4]byte
	DataSize        [4]byte
}

func (ffif *FlatFileInformationFork) MarshalBinary() []byte {
	var b []byte
	b = append(b, ffif.Platform...)
	b = append(b, ffif.TypeSignature...)
	b = append(b, ffif.CreatorSignature...)
	b = append(b, ffif.Flags...)
	b = append(b, ffif.PlatformFlags...)
	b = append(b, ffif.RSVD...)
	b = append(b, ffif.CreateDate...)
	b = append(b, ffif.ModifyDate...)
	b = append(b, ffif.NameScript...)
	b = append(b, ffif.ReadNameSize()...)
	b = append(b, ffif.Name...)
	b = append(b, ffif.CommentSize...)
	b = append(b, ffif.Comment...)

	return b
}

func (ffif *FlatFileInformationFork) UnmarshalBinary(b []byte) error {
	nameSize := b[70:72]
	bs := binary.BigEndian.Uint16(nameSize)
	nameEnd := 72 + bs

	ffif.Platform = b[0:4]
	ffif.TypeSignature = b[4:8]
	ffif.CreatorSignature = b[8:12]
	ffif.Flags = b[12:16]
	ffif.PlatformFlags = b[16:20]
	ffif.RSVD = b[20:52]
	ffif.CreateDate = b[52:60]
	ffif.ModifyDate = b[60:68]
	ffif.NameScript = b[68:70]
	ffif.NameSize = b[70:72]
	ffif.Name = b[72:nameEnd]

	if len(b) > int(nameEnd) {
		ffif.CommentSize = b[nameEnd : nameEnd+2]
		commentLen := binary.BigEndian.Uint16(ffif.CommentSize)

		commentStartPos := int(nameEnd) + 2
		commentEndPos := int(nameEnd) + 2 + int(commentLen)

		ffif.Comment = b[commentStartPos:commentEndPos]
	}

	return nil
}

func (ffo *flattenedFileObject) BinaryMarshal() []byte {
	var out []byte
	out = append(out, ffo.FlatFileHeader.Format[:]...)
	out = append(out, ffo.FlatFileHeader.Version[:]...)
	out = append(out, ffo.FlatFileHeader.RSVD[:]...)
	out = append(out, ffo.FlatFileHeader.ForkCount[:]...)

	out = append(out, []byte("INFO")...)
	out = append(out, []byte{0, 0, 0, 0}...)
	out = append(out, make([]byte, 4)...)
	out = append(out, ffo.FlatFileInformationFork.DataSize()...)

	out = append(out, ffo.FlatFileInformationFork.Platform...)
	out = append(out, ffo.FlatFileInformationFork.TypeSignature...)
	out = append(out, ffo.FlatFileInformationFork.CreatorSignature...)
	out = append(out, ffo.FlatFileInformationFork.Flags...)
	out = append(out, ffo.FlatFileInformationFork.PlatformFlags...)
	out = append(out, ffo.FlatFileInformationFork.RSVD...)
	out = append(out, ffo.FlatFileInformationFork.CreateDate...)
	out = append(out, ffo.FlatFileInformationFork.ModifyDate...)
	out = append(out, ffo.FlatFileInformationFork.NameScript...)
	out = append(out, ffo.FlatFileInformationFork.ReadNameSize()...)
	out = append(out, ffo.FlatFileInformationFork.Name...)
	out = append(out, ffo.FlatFileInformationFork.CommentSize...)
	out = append(out, ffo.FlatFileInformationFork.Comment...)

	out = append(out, ffo.FlatFileDataForkHeader.ForkType[:]...)
	out = append(out, ffo.FlatFileDataForkHeader.CompressionType[:]...)
	out = append(out, ffo.FlatFileDataForkHeader.RSVD[:]...)
	out = append(out, ffo.FlatFileDataForkHeader.DataSize[:]...)

	return out
}

func (ffo *flattenedFileObject) ReadFrom(r io.Reader) (int, error) {
	var n int

	if err := binary.Read(r, binary.BigEndian, &ffo.FlatFileHeader); err != nil {
		return n, err
	}

	if err := binary.Read(r, binary.BigEndian, &ffo.FlatFileInformationForkHeader); err != nil {
		return n, err
	}

	dataLen := binary.BigEndian.Uint32(ffo.FlatFileInformationForkHeader.DataSize[:])
	ffifBuf := make([]byte, dataLen)
	if _, err := io.ReadFull(r, ffifBuf); err != nil {
		return n, err
	}

	if err := ffo.FlatFileInformationFork.UnmarshalBinary(ffifBuf); err != nil {
		return n, err
	}

	if err := binary.Read(r, binary.BigEndian, &ffo.FlatFileDataForkHeader); err != nil {
		return n, err
	}

	return n, nil
}

func (ffo *flattenedFileObject) dataSize() int64 {
	return int64(binary.BigEndian.Uint32(ffo.FlatFileDataForkHeader.DataSize[:]))
}

func (ffo *flattenedFileObject) rsrcSize() int64 {
	return int64(binary.BigEndian.Uint32(ffo.FlatFileResForkHeader.DataSize[:]))
}
