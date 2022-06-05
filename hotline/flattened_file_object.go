package hotline

import (
	"encoding/binary"
	"os"
)

type flattenedFileObject struct {
	FlatFileHeader                FlatFileHeader
	FlatFileInformationForkHeader FlatFileInformationForkHeader
	FlatFileInformationFork       FlatFileInformationFork
	FlatFileDataForkHeader        FlatFileDataForkHeader
	FileData                      []byte
}

// FlatFileHeader is the first section of a "Flattened File Object".  All fields have static values.
type FlatFileHeader struct {
	Format    [4]byte  // Always "FILP"
	Version   [2]byte  // Always 1
	RSVD      [16]byte // Always empty zeros
	ForkCount [2]byte  // Number of forks
}

// NewFlatFileHeader returns a FlatFileHeader struct
func NewFlatFileHeader() FlatFileHeader {
	return FlatFileHeader{
		Format:    [4]byte{0x46, 0x49, 0x4c, 0x50}, // FILP
		Version:   [2]byte{0, 1},
		RSVD:      [16]byte{},
		ForkCount: [2]byte{0, 2},
	}
}

// FlatFileInformationForkHeader is the second section of a "Flattened File Object"
type FlatFileInformationForkHeader struct {
	ForkType        [4]byte // Always "INFO"
	CompressionType [4]byte // Always 0; Compression was never implemented in the Hotline protocol
	RSVD            [4]byte // Always zeros
	DataSize        [4]byte // Size of the flat file information fork
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
	NameScript       []byte // TODO: what is this?
	NameSize         []byte // Length of file name (Maximum 128 characters)
	Name             []byte // File name
	CommentSize      []byte // Length of file comment
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
	return ffif.CreatorSignature
}

// DataSize calculates the size of the flat file information fork, which is
// 72 bytes for the fixed length fields plus the length of the Name + Comment
func (ffif *FlatFileInformationFork) DataSize() []byte {
	size := make([]byte, 4)

	// TODO: Can I do math directly on two byte slices?
	dataSize := len(ffif.Name) + len(ffif.Comment) + 74

	binary.BigEndian.PutUint32(size, uint32(dataSize))

	return size
}

func (ffo *flattenedFileObject) TransferSize() []byte {
	payloadSize := len(ffo.BinaryMarshal())
	dataSize := binary.BigEndian.Uint32(ffo.FlatFileDataForkHeader.DataSize[:])

	transferSize := make([]byte, 4)
	binary.BigEndian.PutUint32(transferSize, dataSize+uint32(payloadSize))

	return transferSize
}

func (ffif *FlatFileInformationFork) ReadNameSize() []byte {
	size := make([]byte, 2)
	binary.BigEndian.PutUint16(size, uint16(len(ffif.Name)))

	return size
}

type FlatFileDataForkHeader struct {
	ForkType        [4]byte
	CompressionType [4]byte
	RSVD            [4]byte
	DataSize        [4]byte
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

func (f *flattenedFileObject) BinaryMarshal() []byte {
	var out []byte
	out = append(out, f.FlatFileHeader.Format[:]...)
	out = append(out, f.FlatFileHeader.Version[:]...)
	out = append(out, f.FlatFileHeader.RSVD[:]...)
	out = append(out, f.FlatFileHeader.ForkCount[:]...)

	out = append(out, []byte("INFO")...)
	out = append(out, []byte{0, 0, 0, 0}...)
	out = append(out, make([]byte, 4)...)
	out = append(out, f.FlatFileInformationFork.DataSize()...)

	out = append(out, f.FlatFileInformationFork.Platform...)
	out = append(out, f.FlatFileInformationFork.TypeSignature...)
	out = append(out, f.FlatFileInformationFork.CreatorSignature...)
	out = append(out, f.FlatFileInformationFork.Flags...)
	out = append(out, f.FlatFileInformationFork.PlatformFlags...)
	out = append(out, f.FlatFileInformationFork.RSVD...)
	out = append(out, f.FlatFileInformationFork.CreateDate...)
	out = append(out, f.FlatFileInformationFork.ModifyDate...)
	out = append(out, f.FlatFileInformationFork.NameScript...)
	out = append(out, f.FlatFileInformationFork.ReadNameSize()...)
	out = append(out, f.FlatFileInformationFork.Name...)
	out = append(out, f.FlatFileInformationFork.CommentSize...)
	out = append(out, f.FlatFileInformationFork.Comment...)

	out = append(out, f.FlatFileDataForkHeader.ForkType[:]...)
	out = append(out, f.FlatFileDataForkHeader.CompressionType[:]...)
	out = append(out, f.FlatFileDataForkHeader.RSVD[:]...)
	out = append(out, f.FlatFileDataForkHeader.DataSize[:]...)

	return out
}

func NewFlattenedFileObject(fileRoot string, filePath, fileName []byte, dataOffset int64) (*flattenedFileObject, error) {
	fullFilePath, err := readPath(fileRoot, filePath, fileName)
	if err != nil {
		return nil, err
	}
	file, err := effectiveFile(fullFilePath)
	if err != nil {
		return nil, err
	}

	defer func(file *os.File) { _ = file.Close() }(file)

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}

	dataSize := make([]byte, 4)
	binary.BigEndian.PutUint32(dataSize, uint32(fileInfo.Size()-dataOffset))

	mTime := toHotlineTime(fileInfo.ModTime())

	ft, _ := fileTypeFromInfo(fileInfo)

	return &flattenedFileObject{
		FlatFileHeader:          NewFlatFileHeader(),
		FlatFileInformationFork: NewFlatFileInformationFork(string(fileName), mTime, ft.TypeCode, ft.CreatorCode),
		FlatFileDataForkHeader: FlatFileDataForkHeader{
			ForkType:        [4]byte{0x44, 0x41, 0x54, 0x41}, // "DATA"
			CompressionType: [4]byte{},
			RSVD:            [4]byte{},
			DataSize:        [4]byte{dataSize[0], dataSize[1], dataSize[2], dataSize[3]},
		},
	}, nil
}
