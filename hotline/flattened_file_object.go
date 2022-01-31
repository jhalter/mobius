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
	ForkCount [2]byte  // Always 2
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
	ForkType        []byte // Always "INFO"
	CompressionType []byte // Always 0; Compression was never implemented in the Hotline protocol
	RSVD            []byte // Always zeros
	DataSize        []byte // Size of the flat file information fork
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

func NewFlatFileInformationFork(fileName string) FlatFileInformationFork {
	return FlatFileInformationFork{
		Platform:         []byte("AMAC"),                                         // TODO: Remove hardcode to support "AWIN" Platform (maybe?)
		TypeSignature:    []byte(fileTypeFromFilename(fileName)),                 // TODO: Don't infer types from filename
		CreatorSignature: []byte(fileCreatorFromFilename(fileName)),              // TODO: Don't infer types from filename
		Flags:            []byte{0, 0, 0, 0},                                     // TODO: What is this?
		PlatformFlags:    []byte{0, 0, 1, 0},                                     // TODO: What is this?
		RSVD:             make([]byte, 32),                                       // Unimplemented in Hotline Protocol
		CreateDate:       []byte{0x07, 0x70, 0x00, 0x00, 0xba, 0x74, 0x24, 0x73}, // TODO: implement
		ModifyDate:       []byte{0x07, 0x70, 0x00, 0x00, 0xba, 0x74, 0x24, 0x73}, // TODO: implement
		NameScript:       make([]byte, 2),                                        // TODO: What is this?
		Name:             []byte(fileName),
		Comment:          []byte("TODO"), // TODO: implement (maybe?)
	}
}

// Size of the flat file information fork, which is the fixed size of 72 bytes
// plus the number of bytes in the FileName
// TODO: plus the size of the Comment!
func (ffif FlatFileInformationFork) DataSize() []byte {
	size := make([]byte, 4)
	nameLen := len(ffif.Name)
	//TODO: Can I do math directly on two byte slices?
	dataSize := nameLen + 74

	binary.BigEndian.PutUint32(size, uint32(dataSize))

	return size
}

func (ffo flattenedFileObject) TransferSize() []byte {
	payloadSize := len(ffo.BinaryMarshal())
	dataSize := binary.BigEndian.Uint32(ffo.FlatFileDataForkHeader.DataSize)

	transferSize := make([]byte, 4)
	binary.BigEndian.PutUint32(transferSize, dataSize+uint32(payloadSize))

	return transferSize
}

func (ffif FlatFileInformationFork) ReadNameSize() []byte {
	size := make([]byte, 2)
	binary.BigEndian.PutUint16(size, uint16(len(ffif.Name)))

	return size
}

type FlatFileDataForkHeader struct {
	ForkType        []byte
	CompressionType []byte
	RSVD            []byte
	DataSize        []byte
}

// ReadFlattenedFileObject parses a byte slice into a flattenedFileObject
func ReadFlattenedFileObject(bytes []byte) flattenedFileObject {
	nameSize := bytes[110:112]
	bs := binary.BigEndian.Uint16(nameSize)

	nameEnd := 112 + bs

	commentSize := bytes[nameEnd : nameEnd+2]
	commentLen := binary.BigEndian.Uint16(commentSize)

	commentStartPos := int(nameEnd) + 2
	commentEndPos := int(nameEnd) + 2 + int(commentLen)

	comment := bytes[commentStartPos:commentEndPos]

	//dataSizeField := bytes[nameEnd+14+commentLen : nameEnd+18+commentLen]
	//dataSize := binary.BigEndian.Uint32(dataSizeField)

	ffo := flattenedFileObject{
		FlatFileHeader: NewFlatFileHeader(),
		FlatFileInformationForkHeader: FlatFileInformationForkHeader{
			ForkType:        bytes[24:28],
			CompressionType: bytes[28:32],
			RSVD:            bytes[32:36],
			DataSize:        bytes[36:40],
		},
		FlatFileInformationFork: FlatFileInformationFork{
			Platform:         bytes[40:44],
			TypeSignature:    bytes[44:48],
			CreatorSignature: bytes[48:52],
			Flags:            bytes[52:56],
			PlatformFlags:    bytes[56:60],
			RSVD:             bytes[60:92],
			CreateDate:       bytes[92:100],
			ModifyDate:       bytes[100:108],
			NameScript:       bytes[108:110],
			NameSize:         bytes[110:112],
			Name:             bytes[112:nameEnd],
			CommentSize:      bytes[nameEnd : nameEnd+2],
			Comment:          comment,
		},
		FlatFileDataForkHeader: FlatFileDataForkHeader{
			ForkType:        bytes[commentEndPos : commentEndPos+4],
			CompressionType: bytes[commentEndPos+4 : commentEndPos+8],
			RSVD:            bytes[commentEndPos+8 : commentEndPos+12],
			DataSize:        bytes[commentEndPos+12 : commentEndPos+16],
		},
	}

	return ffo
}

func (f flattenedFileObject) BinaryMarshal() []byte {
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

	// TODO: Implement commentlen and comment field
	out = append(out, []byte{0, 0}...)

	out = append(out, f.FlatFileDataForkHeader.ForkType...)
	out = append(out, f.FlatFileDataForkHeader.CompressionType...)
	out = append(out, f.FlatFileDataForkHeader.RSVD...)
	out = append(out, f.FlatFileDataForkHeader.DataSize...)

	return out
}

func NewFlattenedFileObject(fileRoot string, filePath, fileName []byte) (*flattenedFileObject, error) {
	fullFilePath, err := readPath(fileRoot, filePath, fileName)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(fullFilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}

	dataSize := make([]byte, 4)
	binary.BigEndian.PutUint32(dataSize, uint32(fileInfo.Size()))

	return &flattenedFileObject{
		FlatFileHeader:          NewFlatFileHeader(),
		FlatFileInformationFork: NewFlatFileInformationFork(string(fileName)),
		FlatFileDataForkHeader: FlatFileDataForkHeader{
			ForkType:        []byte("DATA"),
			CompressionType: []byte{0, 0, 0, 0},
			RSVD:            []byte{0, 0, 0, 0},
			DataSize:        dataSize,
		},
	}, nil
}
