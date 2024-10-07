package hotline

type fileType struct {
	TypeCode    [4]byte // 4 byte type code used in file transfers
	CreatorCode [4]byte // 4 byte creator code used in file transfers
}

func newFileType(typeCode, creatorCode string) fileType {
	return fileType{
		TypeCode:    [4]byte{typeCode[0], typeCode[1], typeCode[2], typeCode[3]},
		CreatorCode: [4]byte{creatorCode[0], creatorCode[1], creatorCode[2], creatorCode[3]},
	}
}

var defaultFileType = newFileType("TEXT", "TTXT")

var fileTypes = map[string]fileType{
	".sit":        newFileType("SIT!", "SIT!"),
	".pdf":        newFileType("PDF ", "CARO"),
	".gif":        newFileType("GIFf", "ogle"),
	".txt":        newFileType("TEXT", "ttxt"),
	".zip":        newFileType("ZIP ", "SITx"),
	".tgz":        newFileType("Gzip", "SITx"),
	".hqx":        newFileType("TEXT", "SITx"),
	".jpg":        newFileType("JPEG", "ogle"),
	".jpeg":       newFileType("JPEG", "ogle"),
	".img":        newFileType("rohd", "ddsk"),
	".sea":        newFileType("APPL", "aust"),
	".mov":        newFileType("MooV", "TVOD"),
	".incomplete": newFileType("HTft", "HTLC"), // Partial file upload
}

// A small number of type codes are displayed in the GetInfo window with a friendly name instead of the 4 letter code
var friendlyCreatorNames = map[string]string{
	"APPL": "Application Program",
	"HTbm": "Hotline Bookmark",
	"fldr": "Folder",
	"flda": "Folder Alias",
	"HTft": "Incomplete File",
	"SIT!": "StuffIt Archive",
	"TEXT": "Text File",
	"HTLC": "Hotline",
}
