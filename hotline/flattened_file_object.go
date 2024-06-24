package hotline

import (
	"bytes"
	"encoding/binary"
	"io"
	"slices"
)

type flattenedFileObject struct {
	FlatFileHeader                FlatFileHeader
	FlatFileInformationForkHeader FlatFileForkHeader
	FlatFileInformationFork       FlatFileInformationFork
	FlatFileDataForkHeader        FlatFileForkHeader
	FlatFileResForkHeader         FlatFileForkHeader

	readOffset int // Internal offset to track read progress
}

// FlatFileHeader is the first section of a "Flattened File Object".  All fields have static values.
type FlatFileHeader struct {
	Format    [4]byte  // Always "FILP"
	Version   [2]byte  // Always 1
	RSVD      [16]byte // Always empty zeros
	ForkCount [2]byte  // Number of forks, either 2 or 3 if there is a resource fork
}

type FlatFileInformationFork struct {
	Platform         [4]byte // Operating System used. ("AMAC" or "MWIN")
	TypeSignature    [4]byte // File type signature
	CreatorSignature [4]byte // File creator signature
	Flags            [4]byte
	PlatformFlags    [4]byte
	RSVD             [32]byte
	CreateDate       [8]byte
	ModifyDate       [8]byte
	NameScript       [2]byte
	NameSize         [2]byte // Length of file name (Maximum 128 characters)
	Name             []byte  // File name
	CommentSize      [2]byte // Length of the comment
	Comment          []byte  // File comment

	readOffset int // Internal offset to track read progress
}

func NewFlatFileInformationFork(fileName string, modifyTime [8]byte, typeSignature string, creatorSignature string) FlatFileInformationFork {
	return FlatFileInformationFork{
		Platform:         [4]byte{0x41, 0x4D, 0x41, 0x43},   // "AMAC" TODO: Remove hardcode to support "AWIN" Platform (maybe?)
		TypeSignature:    [4]byte([]byte(typeSignature)),    // TODO: Don't infer types from filename
		CreatorSignature: [4]byte([]byte(creatorSignature)), // TODO: Don't infer types from filename
		PlatformFlags:    [4]byte{0, 0, 1, 0},               // TODO: What is this?
		CreateDate:       modifyTime,                        // some filesystems don't support createTime
		ModifyDate:       modifyTime,
		Name:             []byte(fileName),
		Comment:          []byte{}, // TODO: implement (maybe?)
	}
}

func (ffif *FlatFileInformationFork) friendlyType() []byte {
	if name, ok := friendlyCreatorNames[string(ffif.TypeSignature[:])]; ok {
		return []byte(name)
	}
	return ffif.TypeSignature[:]
}

func (ffif *FlatFileInformationFork) friendlyCreator() []byte {
	if name, ok := friendlyCreatorNames[string(ffif.CreatorSignature[:])]; ok {
		return []byte(name)
	}
	return ffif.CreatorSignature[:]
}

func (ffif *FlatFileInformationFork) setComment(comment []byte) error {
	commentSize := make([]byte, 2)
	ffif.Comment = comment
	binary.BigEndian.PutUint16(commentSize, uint16(len(comment)))
	ffif.CommentSize = [2]byte(commentSize)
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
	ffoCopy := *ffo

	// get length of the flattenedFileObject, including the info fork
	b, _ := io.ReadAll(&ffoCopy)
	payloadSize := len(b)

	// length of data fork
	dataSize := binary.BigEndian.Uint32(ffo.FlatFileDataForkHeader.DataSize[:])

	// length of resource fork
	resForkSize := binary.BigEndian.Uint32(ffo.FlatFileResForkHeader.DataSize[:])

	size := make([]byte, 4)
	binary.BigEndian.PutUint32(size, dataSize+resForkSize+uint32(payloadSize)-uint32(offset))

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

func (ffif *FlatFileInformationFork) Read(p []byte) (int, error) {
	buf := slices.Concat(
		ffif.Platform[:],
		ffif.TypeSignature[:],
		ffif.CreatorSignature[:],
		ffif.Flags[:],
		ffif.PlatformFlags[:],
		ffif.RSVD[:],
		ffif.CreateDate[:],
		ffif.ModifyDate[:],
		ffif.NameScript[:],
		ffif.ReadNameSize(),
		ffif.Name,
		ffif.CommentSize[:],
		ffif.Comment,
	)

	if ffif.readOffset >= len(buf) {
		return 0, io.EOF // All bytes have been read
	}

	n := copy(p, buf[ffif.readOffset:])
	ffif.readOffset += n

	return n, nil
}

// Write implements the io.Writer interface for FlatFileInformationFork
func (ffif *FlatFileInformationFork) Write(p []byte) (int, error) {
	nameSize := p[70:72]
	bs := binary.BigEndian.Uint16(nameSize)
	total := 72 + bs

	ffif.Platform = [4]byte(p[0:4])
	ffif.TypeSignature = [4]byte(p[4:8])
	ffif.CreatorSignature = [4]byte(p[8:12])
	ffif.Flags = [4]byte(p[12:16])
	ffif.PlatformFlags = [4]byte(p[16:20])
	ffif.RSVD = [32]byte(p[20:52])
	ffif.CreateDate = [8]byte(p[52:60])
	ffif.ModifyDate = [8]byte(p[60:68])
	ffif.NameScript = [2]byte(p[68:70])
	ffif.NameSize = [2]byte(p[70:72])
	ffif.Name = p[72:total]

	if len(p) > int(total) {
		ffif.CommentSize = [2]byte(p[total : total+2])
		commentLen := binary.BigEndian.Uint16(ffif.CommentSize[:])
		commentStartPos := int(total) + 2
		commentEndPos := int(total) + 2 + int(commentLen)

		ffif.Comment = p[commentStartPos:commentEndPos]

		//total = uint16(commentEndPos)
	}

	return len(p), nil
}

func (ffif *FlatFileInformationFork) UnmarshalBinary(b []byte) error {
	nameSize := b[70:72]
	bs := binary.BigEndian.Uint16(nameSize)
	nameEnd := 72 + bs

	ffif.Platform = [4]byte(b[0:4])
	ffif.TypeSignature = [4]byte(b[4:8])
	ffif.CreatorSignature = [4]byte(b[8:12])
	ffif.Flags = [4]byte(b[12:16])
	ffif.PlatformFlags = [4]byte(b[16:20])
	ffif.RSVD = [32]byte(b[20:52])
	ffif.CreateDate = [8]byte(b[52:60])
	ffif.ModifyDate = [8]byte(b[60:68])
	ffif.NameScript = [2]byte(b[68:70])
	ffif.NameSize = [2]byte(b[70:72])
	ffif.Name = b[72:nameEnd]

	if len(b) > int(nameEnd) {
		ffif.CommentSize = [2]byte(b[nameEnd : nameEnd+2])
		commentLen := binary.BigEndian.Uint16(ffif.CommentSize[:])

		commentStartPos := int(nameEnd) + 2
		commentEndPos := int(nameEnd) + 2 + int(commentLen)

		ffif.Comment = b[commentStartPos:commentEndPos]
	}

	return nil
}

// Read implements the io.Reader interface for flattenedFileObject
func (ffo *flattenedFileObject) Read(p []byte) (int, error) {
	buf := slices.Concat(
		ffo.FlatFileHeader.Format[:],
		ffo.FlatFileHeader.Version[:],
		ffo.FlatFileHeader.RSVD[:],
		ffo.FlatFileHeader.ForkCount[:],
		[]byte("INFO"),
		[]byte{0, 0, 0, 0},
		make([]byte, 4),
		ffo.FlatFileInformationFork.DataSize(),
		ffo.FlatFileInformationFork.Platform[:],
		ffo.FlatFileInformationFork.TypeSignature[:],
		ffo.FlatFileInformationFork.CreatorSignature[:],
		ffo.FlatFileInformationFork.Flags[:],
		ffo.FlatFileInformationFork.PlatformFlags[:],
		ffo.FlatFileInformationFork.RSVD[:],
		ffo.FlatFileInformationFork.CreateDate[:],
		ffo.FlatFileInformationFork.ModifyDate[:],
		ffo.FlatFileInformationFork.NameScript[:],
		ffo.FlatFileInformationFork.ReadNameSize(),
		ffo.FlatFileInformationFork.Name,
		ffo.FlatFileInformationFork.CommentSize[:],
		ffo.FlatFileInformationFork.Comment,
		ffo.FlatFileDataForkHeader.ForkType[:],
		ffo.FlatFileDataForkHeader.CompressionType[:],
		ffo.FlatFileDataForkHeader.RSVD[:],
		ffo.FlatFileDataForkHeader.DataSize[:],
	)

	if ffo.readOffset >= len(buf) {
		return 0, io.EOF // All bytes have been read
	}

	n := copy(p, buf[ffo.readOffset:])
	ffo.readOffset += n

	return n, nil
}

func (ffo *flattenedFileObject) ReadFrom(r io.Reader) (int64, error) {
	var n int64

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

	_, err := io.Copy(&ffo.FlatFileInformationFork, bytes.NewReader(ffifBuf))
	if err != nil {
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
