package hotline

const (
	accessDeleteFile       = 0  // File System Maintenance: Can Delete Files
	accessUploadFile       = 1  // File System Maintenance: Can Upload Files
	accessDownloadFile     = 2  // File System Maintenance: Can Download Files
	accessRenameFile       = 3  // File System Maintenance: Can Rename Files
	accessMoveFile         = 4  // File System Maintenance: Can Move Files
	accessCreateFolder     = 5  // File System Maintenance: Can Create Folders
	accessDeleteFolder     = 6  // File System Maintenance: Can Delete Folders
	accessRenameFolder     = 7  // File System Maintenance: Can Rename Folders
	accessMoveFolder       = 8  // File System Maintenance: Can Move Folders
	accessReadChat         = 9  // Chat: Can Read Chat
	accessSendChat         = 10 // Chat: Can Send Chat
	accessOpenChat         = 11 // Chat: Can Initial Private Chat
	accessCloseChat        = 12 // Present in the Hotline 1.9 protocol documentation, but seemingly unused
	accessShowInList       = 13 // Present in the Hotline 1.9 protocol documentation, but seemingly unused
	accessCreateUser       = 14 // User Maintenance: Can Create Accounts
	accessDeleteUser       = 15 // User Maintenance: Can Delete Accounts
	accessOpenUser         = 16 // User Maintenance: Can Read Accounts
	accessModifyUser       = 17 // User Maintenance: Can Modify Accounts
	accessChangeOwnPass    = 18 // Present in the Hotline 1.9 protocol documentation, but seemingly unused
	accessSendPrivMsg      = 19 // Messaging: Can Send Messages
	accessNewsReadArt      = 20 // News: Can Read Articles
	accessNewsPostArt      = 21 // News: Can Post Articles
	accessDisconUser       = 22 // User Maintenance: Can Disconnect Users (Note: Turns username red in user list)
	accessCannotBeDiscon   = 23 // User Maintenance: Cannot be Disconnected
	accessGetClientInfo    = 24 // User Maintenance: Can Get User Info
	accessUploadAnywhere   = 25 // File System Maintenance: Can Upload Anywhere
	accessAnyName          = 26 // Miscellaneous: Can User Any Name
	accessNoAgreement      = 27 // Miscellaneous: Don't Show Agreement
	accessSetFileComment   = 28 // File System Maintenance: Can Comment Files
	accessSetFolderComment = 29 // File System Maintenance: Can Comment Folders
	accessViewDropBoxes    = 30 // File System Maintenance: Can View Drop Boxes
	accessMakeAlias        = 31 // File System Maintenance: Can Make Aliases
	accessBroadcast        = 32 // Messaging: Can Broadcast
	accessNewsDeleteArt    = 33 // News: Can Delete Articles
	accessNewsCreateCat    = 34 // News: Can Create Categories
	accessNewsDeleteCat    = 35 // News: Can Delete Categories
	accessNewsCreateFldr   = 36 // News: Can Create News Bundles
	accessNewsDeleteFldr   = 37 // News: Can Delete News Bundles
)

type accessBitmap [8]byte

func (bits *accessBitmap) Set(i int) {
	bits[i/8] |= 1 << uint(7-i%8)
}

func (bits *accessBitmap) IsSet(i int) bool {
	return bits[i/8]&(1<<uint(7-i%8)) != 0
}
