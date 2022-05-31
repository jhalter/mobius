package hotline

type fileType struct {
	TypeCode    string
	CreatorCode string
}

var defaultFileType = fileType{
	TypeCode:    "TEXT",
	CreatorCode: "TTXT",
}

var fileTypes = map[string]fileType{
	"sit": {
		TypeCode:    "SIT!",
		CreatorCode: "SIT!",
	},
	"pdf": {
		TypeCode:    "PDF ",
		CreatorCode: "CARO",
	},
	"gif": {
		TypeCode:    "GIFf",
		CreatorCode: "ogle",
	},
	"txt": {
		TypeCode:    "TEXT",
		CreatorCode: "ttxt",
	},
	"zip": {
		TypeCode:    "ZIP ",
		CreatorCode: "SITx",
	},
	"tgz": {
		TypeCode:    "Gzip",
		CreatorCode: "SITx",
	},
	"hqx": {
		TypeCode:    "TEXT",
		CreatorCode: "SITx",
	},
	"jpg": {
		TypeCode:    "JPEG",
		CreatorCode: "ogle",
	},
	"img": {
		TypeCode:    "rohd",
		CreatorCode: "ddsk",
	},
	"sea": {
		TypeCode:    "APPL",
		CreatorCode: "aust",
	},
	"mov": {
		TypeCode:    "MooV",
		CreatorCode: "TVOD",
	},
}
