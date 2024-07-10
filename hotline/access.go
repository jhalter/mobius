package hotline

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
	AccessSendPrivMsg      = 40 // Messaging: Can Send Messages (Note: 1.9 protocol doc incorrectly says this is bit 19)
)

type accessBitmap [8]byte

func (bits *accessBitmap) Set(i int) {
	bits[i/8] |= 1 << uint(7-i%8)
}

func (bits *accessBitmap) IsSet(i int) bool {
	return bits[i/8]&(1<<uint(7-i%8)) != 0
}
