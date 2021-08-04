package hotline

import (
	"encoding/binary"
	"math/big"
)

const (
	accessAlwaysAllow = -1 // Some transactions are always allowed

	// File System Maintenance
	accessDeleteFile   = 0
	accessUploadFile   = 1
	accessDownloadFile = 2 // Can Download Files
	accessRenameFile   = 3
	accessMoveFile     = 4
	accessCreateFolder = 5
	accessDeleteFolder = 6
	accessRenameFolder = 7
	accessMoveFolder   = 8
	accessReadChat     = 9
	accessSendChat     = 10
	accessOpenChat     = 11
	// accessCloseChat        = 12 // Documented but unused?
	// accessShowInList       = 13 // Documented but unused?
	accessCreateUser = 14
	accessDeleteUser = 15
	accessOpenUser   = 16
	accessModifyUser = 17
	// accessChangeOwnPass    = 18 // Documented but unused?
	accessSendPrivMsg      = 19 // This doesn't do what it seems like it should do. TODO: Investigate
	accessNewsReadArt      = 20
	accessNewsPostArt      = 21
	accessDisconUser       = 22 // Toggles red user name in user list
	accessCannotBeDiscon   = 23
	accessGetClientInfo    = 24
	accessUploadAnywhere   = 25
	accessAnyName          = 26
	accessNoAgreement      = 27
	accessSetFileComment   = 28
	accessSetFolderComment = 29
	accessViewDropBoxes    = 30
	accessMakeAlias        = 31
	accessBroadcast        = 32
	accessNewsDeleteArt    = 33
	accessNewsCreateCat    = 34
	accessNewsDeleteCat    = 35
	accessNewsCreateFldr   = 36
	accessNewsDeleteFldr   = 37
)

func authorize(access *[]byte, accessBit int) bool {
	if accessBit == accessAlwaysAllow {
		return true
	}
	accessBitmap := big.NewInt(int64(binary.BigEndian.Uint64(*access)))

	return accessBitmap.Bit(63-accessBit) == 1
}
