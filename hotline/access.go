package hotline

import "fmt"

const (
	AccessDeleteFile       = 0  // File System Maintenance: Can Delete Files
	AccessUploadFile       = 1  // File System Maintenance: Can Upload Files
	AccessDownloadFile     = 2  // File System Maintenance: Can Download Files
	AccessRenameFile       = 3  // File System Maintenance: Can Rename Files
	AccessMoveFile         = 4  // File System Maintenance: Can Move Files
	AccessCreateFolder     = 5  // File System Maintenance: Can Create Folders
	AccessDeleteFolder     = 6  // File System Maintenance: Can Delete Folders
	AccessRenameFolder     = 7  // File System Maintenance: Can Rename Folders
	AccessMoveFolder       = 8  // File System Maintenance: Can Move Folders
	AccessReadChat         = 9  // Chat: Can Read Chat
	AccessSendChat         = 10 // Chat: Can Send Chat
	AccessOpenChat         = 11 // Chat: Can Initial Private Chat
	AccessCloseChat        = 12 // Present in the Hotline 1.9 protocol documentation, but seemingly unused
	AccessShowInList       = 13 // Present in the Hotline 1.9 protocol documentation, but seemingly unused
	AccessCreateUser       = 14 // User Maintenance: Can Create Accounts
	AccessDeleteUser       = 15 // User Maintenance: Can Delete Accounts
	AccessOpenUser         = 16 // User Maintenance: Can Read Accounts
	AccessModifyUser       = 17 // User Maintenance: Can Modify Accounts
	AccessChangeOwnPass    = 18 // Present in the Hotline 1.9 protocol documentation, but seemingly unused
	AccessNewsReadArt      = 20 // News: Can Read Articles
	AccessNewsPostArt      = 21 // News: Can Post Articles
	AccessDisconUser       = 22 // User Maintenance: Can Disconnect Users (Note: Turns username red in user list)
	AccessCannotBeDiscon   = 23 // User Maintenance: Cannot be Disconnected
	AccessGetClientInfo    = 24 // User Maintenance: Can Get User Info
	AccessUploadAnywhere   = 25 // File System Maintenance: Can Upload Anywhere
	AccessAnyName          = 26 // Miscellaneous: Can User Any Name
	AccessNoAgreement      = 27 // Miscellaneous: Don't Show Agreement
	AccessSetFileComment   = 28 // File System Maintenance: Can Comment Files
	AccessSetFolderComment = 29 // File System Maintenance: Can Comment Folders
	AccessViewDropBoxes    = 30 // File System Maintenance: Can View Drop Boxes
	AccessMakeAlias        = 31 // File System Maintenance: Can Make Aliases
	AccessBroadcast        = 32 // Messaging: Can Broadcast
	AccessNewsDeleteArt    = 33 // News: Can Delete Articles
	AccessNewsCreateCat    = 34 // News: Can Create Categories
	AccessNewsDeleteCat    = 35 // News: Can Delete Categories
	AccessNewsCreateFldr   = 36 // News: Can Create News Bundles
	AccessNewsDeleteFldr   = 37 // News: Can Delete News Bundles
	AccessUploadFolder     = 38 // File System Maintenance: Can Upload Folders
	AccessDownloadFolder   = 39 // File System Maintenance: Can Download Folders
	AccessSendPrivMsg      = 40 // Messaging: Can Send Messages (Note: 1.9 protocol doc incorrectly says this is bit 19)
)

type AccessBitmap [8]byte

func (bits *AccessBitmap) Set(i int) {
	bits[i/8] |= 1 << uint(7-i%8)
}

func (bits *AccessBitmap) IsSet(i int) bool {
	return bits[i/8]&(1<<uint(7-i%8)) != 0
}

func (bits *AccessBitmap) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var flags interface{}
	err := unmarshal(&flags)
	if err != nil {
		return fmt.Errorf("unmarshal access bitmap: %w", err)
	}

	switch v := flags.(type) {
	case []interface{}:
		// Mobius versions < v0.17.0 store the user access bitmap as an array of int values like:
		// [96, 112, 12, 32, 3, 128, 0, 0]
		// This case supports reading of user config files using this format.
		for i, v := range flags.([]interface{}) {
			bits[i] = byte(v.(int))
		}
	case map[string]interface{}:
		// Mobius versions >= v0.17.0 store the user access bitmap as map[string]bool to provide a human-readable view of
		// the account permissions.
		if f, ok := v["DeleteFile"].(bool); ok && f {
			bits.Set(AccessDeleteFile)
		}
		if f, ok := v["UploadFile"].(bool); ok && f {
			bits.Set(AccessUploadFile)
		}
		if f, ok := v["DownloadFile"].(bool); ok && f {
			bits.Set(AccessDownloadFile)
		}
		if f, ok := v["UploadFolder"].(bool); ok && f {
			bits.Set(AccessUploadFolder)
		}
		if f, ok := v["DownloadFolder"].(bool); ok && f {
			bits.Set(AccessDownloadFolder)
		}
		if f, ok := v["RenameFile"].(bool); ok && f {
			bits.Set(AccessRenameFile)
		}
		if f, ok := v["MoveFile"].(bool); ok && f {
			bits.Set(AccessMoveFile)
		}
		if f, ok := v["CreateFolder"].(bool); ok && f {
			bits.Set(AccessCreateFolder)
		}
		if f, ok := v["DeleteFolder"].(bool); ok && f {
			bits.Set(AccessDeleteFolder)
		}
		if f, ok := v["RenameFolder"].(bool); ok && f {
			bits.Set(AccessRenameFolder)
		}
		if f, ok := v["MoveFolder"].(bool); ok && f {
			bits.Set(AccessMoveFolder)
		}
		if f, ok := v["ReadChat"].(bool); ok && f {
			bits.Set(AccessReadChat)
		}
		if f, ok := v["SendChat"].(bool); ok && f {
			bits.Set(AccessSendChat)
		}
		if f, ok := v["OpenChat"].(bool); ok && f {
			bits.Set(AccessOpenChat)
		}
		if f, ok := v["CloseChat"].(bool); ok && f {
			bits.Set(AccessCloseChat)
		}
		if f, ok := v["ShowInList"].(bool); ok && f {
			bits.Set(AccessShowInList)
		}
		if f, ok := v["CreateUser"].(bool); ok && f {
			bits.Set(AccessCreateUser)
		}
		if f, ok := v["DeleteUser"].(bool); ok && f {
			bits.Set(AccessDeleteUser)
		}
		if f, ok := v["OpenUser"].(bool); ok && f {
			bits.Set(AccessOpenUser)
		}
		if f, ok := v["ModifyUser"].(bool); ok && f {
			bits.Set(AccessModifyUser)
		}
		if f, ok := v["ChangeOwnPass"].(bool); ok && f {
			bits.Set(AccessChangeOwnPass)
		}
		if f, ok := v["NewsReadArt"].(bool); ok && f {
			bits.Set(AccessNewsReadArt)
		}
		if f, ok := v["NewsPostArt"].(bool); ok && f {
			bits.Set(AccessNewsPostArt)
		}
		if f, ok := v["DisconnectUser"].(bool); ok && f {
			bits.Set(AccessDisconUser)
		}
		if f, ok := v["CannotBeDisconnected"].(bool); ok && f {
			bits.Set(AccessCannotBeDiscon)
		}
		if f, ok := v["GetClientInfo"].(bool); ok && f {
			bits.Set(AccessGetClientInfo)
		}
		if f, ok := v["UploadAnywhere"].(bool); ok && f {
			bits.Set(AccessUploadAnywhere)
		}
		if f, ok := v["AnyName"].(bool); ok && f {
			bits.Set(AccessAnyName)
		}
		if f, ok := v["NoAgreement"].(bool); ok && f {
			bits.Set(AccessNoAgreement)
		}
		if f, ok := v["SetFileComment"].(bool); ok && f {
			bits.Set(AccessSetFileComment)
		}
		if f, ok := v["SetFolderComment"].(bool); ok && f {
			bits.Set(AccessSetFolderComment)
		}
		if f, ok := v["ViewDropBoxes"].(bool); ok && f {
			bits.Set(AccessViewDropBoxes)
		}
		if f, ok := v["MakeAlias"].(bool); ok && f {
			bits.Set(AccessMakeAlias)
		}
		if f, ok := v["Broadcast"].(bool); ok && f {
			bits.Set(AccessBroadcast)
		}
		if f, ok := v["NewsDeleteArt"].(bool); ok && f {
			bits.Set(AccessNewsDeleteArt)
		}
		if f, ok := v["NewsCreateCat"].(bool); ok && f {
			bits.Set(AccessNewsCreateCat)
		}
		if f, ok := v["NewsDeleteCat"].(bool); ok && f {
			bits.Set(AccessNewsDeleteCat)
		}
		if f, ok := v["NewsCreateFldr"].(bool); ok && f {
			bits.Set(AccessNewsCreateFldr)
		}
		if f, ok := v["NewsDeleteFldr"].(bool); ok && f {
			bits.Set(AccessNewsDeleteFldr)
		}
		if f, ok := v["SendPrivMsg"].(bool); ok && f {
			bits.Set(AccessSendPrivMsg)
		}
	}

	return nil
}

// accessFlags is used to render the access bitmap to human-readable boolean flags in the account yaml.
type accessFlags struct {
	DownloadFile         bool `yaml:"DownloadFile"`
	DownloadFolder       bool `yaml:"DownloadFolder"`
	UploadFile           bool `yaml:"UploadFile"`
	UploadFolder         bool `yaml:"UploadFolder"`
	DeleteFile           bool `yaml:"DeleteFile"`
	RenameFile           bool `yaml:"RenameFile"`
	MoveFile             bool `yaml:"MoveFile"`
	CreateFolder         bool `yaml:"CreateFolder"`
	DeleteFolder         bool `yaml:"DeleteFolder"`
	RenameFolder         bool `yaml:"RenameFolder"`
	MoveFolder           bool `yaml:"MoveFolder"`
	ReadChat             bool `yaml:"ReadChat"`
	SendChat             bool `yaml:"SendChat"`
	OpenChat             bool `yaml:"OpenChat"`
	CloseChat            bool `yaml:"CloseChat"`
	ShowInList           bool `yaml:"ShowInList"`
	CreateUser           bool `yaml:"CreateUser"`
	DeleteUser           bool `yaml:"DeleteUser"`
	OpenUser             bool `yaml:"OpenUser"`
	ModifyUser           bool `yaml:"ModifyUser"`
	ChangeOwnPass        bool `yaml:"ChangeOwnPass"`
	NewsReadArt          bool `yaml:"NewsReadArt"`
	NewsPostArt          bool `yaml:"NewsPostArt"`
	DisconnectUser       bool `yaml:"DisconnectUser"`
	CannotBeDisconnected bool `yaml:"CannotBeDisconnected"`
	GetClientInfo        bool `yaml:"GetClientInfo"`
	UploadAnywhere       bool `yaml:"UploadAnywhere"`
	AnyName              bool `yaml:"AnyName"`
	NoAgreement          bool `yaml:"NoAgreement"`
	SetFileComment       bool `yaml:"SetFileComment"`
	SetFolderComment     bool `yaml:"SetFolderComment"`
	ViewDropBoxes        bool `yaml:"ViewDropBoxes"`
	MakeAlias            bool `yaml:"MakeAlias"`
	Broadcast            bool `yaml:"Broadcast"`
	NewsDeleteArt        bool `yaml:"NewsDeleteArt"`
	NewsCreateCat        bool `yaml:"NewsCreateCat"`
	NewsDeleteCat        bool `yaml:"NewsDeleteCat"`
	NewsCreateFldr       bool `yaml:"NewsCreateFldr"`
	NewsDeleteFldr       bool `yaml:"NewsDeleteFldr"`
	SendPrivMsg          bool `yaml:"SendPrivMsg"`
}

func (bits AccessBitmap) MarshalYAML() (interface{}, error) {
	return accessFlags{
		DownloadFile:         bits.IsSet(AccessDownloadFile),
		DownloadFolder:       bits.IsSet(AccessDownloadFolder),
		UploadFolder:         bits.IsSet(AccessUploadFolder),
		DeleteFile:           bits.IsSet(AccessDeleteFile),
		UploadFile:           bits.IsSet(AccessUploadFile),
		RenameFile:           bits.IsSet(AccessRenameFile),
		MoveFile:             bits.IsSet(AccessMoveFile),
		CreateFolder:         bits.IsSet(AccessCreateFolder),
		DeleteFolder:         bits.IsSet(AccessDeleteFolder),
		RenameFolder:         bits.IsSet(AccessRenameFolder),
		MoveFolder:           bits.IsSet(AccessMoveFolder),
		ReadChat:             bits.IsSet(AccessReadChat),
		SendChat:             bits.IsSet(AccessSendChat),
		OpenChat:             bits.IsSet(AccessOpenChat),
		CloseChat:            bits.IsSet(AccessCloseChat),
		ShowInList:           bits.IsSet(AccessShowInList),
		CreateUser:           bits.IsSet(AccessCreateUser),
		DeleteUser:           bits.IsSet(AccessDeleteUser),
		OpenUser:             bits.IsSet(AccessOpenUser),
		ModifyUser:           bits.IsSet(AccessModifyUser),
		ChangeOwnPass:        bits.IsSet(AccessChangeOwnPass),
		NewsReadArt:          bits.IsSet(AccessNewsReadArt),
		NewsPostArt:          bits.IsSet(AccessNewsPostArt),
		DisconnectUser:       bits.IsSet(AccessDisconUser),
		CannotBeDisconnected: bits.IsSet(AccessCannotBeDiscon),
		GetClientInfo:        bits.IsSet(AccessGetClientInfo),
		UploadAnywhere:       bits.IsSet(AccessUploadAnywhere),
		AnyName:              bits.IsSet(AccessAnyName),
		NoAgreement:          bits.IsSet(AccessNoAgreement),
		SetFileComment:       bits.IsSet(AccessSetFileComment),
		SetFolderComment:     bits.IsSet(AccessSetFolderComment),
		ViewDropBoxes:        bits.IsSet(AccessViewDropBoxes),
		MakeAlias:            bits.IsSet(AccessMakeAlias),
		Broadcast:            bits.IsSet(AccessBroadcast),
		NewsDeleteArt:        bits.IsSet(AccessNewsDeleteArt),
		NewsCreateCat:        bits.IsSet(AccessNewsCreateCat),
		NewsDeleteCat:        bits.IsSet(AccessNewsDeleteCat),
		NewsCreateFldr:       bits.IsSet(AccessNewsCreateFldr),
		NewsDeleteFldr:       bits.IsSet(AccessNewsDeleteFldr),
		SendPrivMsg:          bits.IsSet(AccessSendPrivMsg),
	}, nil
}
