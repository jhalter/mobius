package mobius

import (
	"fmt"
	"io"
	"os"
	"path"

	"github.com/jhalter/mobius/hotline"
)

var fileTypeFLDR = [4]byte{0x66, 0x6c, 0x64, 0x72}

// HandleGetFileInfo requests file information from the server.
//
// Fields used in the request:
//   - 201 File name   Required
//   - 202 File path   Optional
//
// Fields used in the reply:
//   - 201 File name           File name
//   - 205 File type string    Friendly file type description
//   - 206 File creator string Friendly creator description
//   - 210 File comment        Comment string
//   - 213 File type           File type signature
//   - 208 File create date    Creation date
//   - 209 File modify date    Modification date
//   - 207 File size           File size
func HandleGetFileInfo(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	fileName := t.GetField(hotline.FieldFileName).Data
	filePath := t.GetField(hotline.FieldFilePath).Data

	fullFilePath, err := hotline.ReadPath(cc.FileRoot(), filePath, fileName, cc.TextDecoder())
	if err != nil {
		cc.Logger.Error("get file info: read path", "err", err)
		return cc.NewErrReply(t, ErrMsgFileNotFound)
	}

	fw, err := hotline.NewFile(cc.Server.FS, fullFilePath, 0)
	if err != nil {
		cc.Logger.Error("get file info: open file", "err", err)
		return cc.NewErrReply(t, ErrMsgFileNotFound)
	}

	encodedName, err := cc.TextEncoder().String(fw.Name)
	if err != nil {
		cc.Logger.Error("get file info: encode name", "err", err)
		return cc.NewErrReply(t, ErrMsgGetFileInfo)
	}

	fields := []hotline.Field{
		hotline.NewField(hotline.FieldFileName, []byte(encodedName)),
		hotline.NewField(hotline.FieldFileTypeString, fw.Ffo.FlatFileInformationFork.FriendlyType()),
		hotline.NewField(hotline.FieldFileCreatorString, fw.Ffo.FlatFileInformationFork.FriendlyCreator()),
		hotline.NewField(hotline.FieldFileType, fw.Ffo.FlatFileInformationFork.TypeSignature[:]),
		hotline.NewField(hotline.FieldFileCreateDate, fw.Ffo.FlatFileInformationFork.CreateDate[:]),
		hotline.NewField(hotline.FieldFileModifyDate, fw.Ffo.FlatFileInformationFork.ModifyDate[:]),
	}

	// Include the optional FileComment field if there is a comment.
	if len(fw.Ffo.FlatFileInformationFork.Comment) != 0 {
		fields = append(fields, hotline.NewField(hotline.FieldFileComment, fw.Ffo.FlatFileInformationFork.Comment))
	}

	// Include the FileSize field for files.
	if fw.Ffo.FlatFileInformationFork.TypeSignature != fileTypeFLDR {
		fields = append(fields, hotline.NewField(hotline.FieldFileSize, fw.TotalSize()))
	}

	return append(res, cc.NewReply(t, fields...))
}

// HandleSetFileInfo sets information for the specified file on the server.
//
// Access: Set File Comment (28) or Set Folder Comment (29)
//
// Fields used in the request:
//   - 201 File name      Required
//   - 202 File path      Optional
//   - 211 File new name  Optional
//   - 210 File comment   Optional
//
// Fields used in the reply: None
func HandleSetFileInfo(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	fileName := t.GetField(hotline.FieldFileName).Data
	filePath := t.GetField(hotline.FieldFilePath).Data

	fullFilePath, err := hotline.ReadPath(cc.FileRoot(), filePath, fileName, cc.TextDecoder())
	if err != nil {
		cc.Logger.Error("set file info: read path", "err", err)
		return cc.NewErrReply(t, ErrMsgFileNotFound)
	}

	fi, err := cc.Server.FS.Stat(fullFilePath)
	if err != nil {
		cc.Logger.Error("set file info: stat file", "err", err)
		return cc.NewErrReply(t, ErrMsgFileNotFound)
	}

	hlFile, err := hotline.NewFile(cc.Server.FS, fullFilePath, 0)
	if err != nil {
		cc.Logger.Error("set file info: open file", "err", err)
		return cc.NewErrReply(t, ErrMsgFileNotFound)
	}
	if t.GetField(hotline.FieldFileComment).Data != nil {
		switch mode := fi.Mode(); {
		case mode.IsDir():
			if !cc.Authorize(hotline.AccessSetFolderComment) {
				return cc.NewErrReply(t, ErrMsgNotAllowedSetCommentsFolders)
			}
		case mode.IsRegular():
			if !cc.Authorize(hotline.AccessSetFileComment) {
				return cc.NewErrReply(t, ErrMsgNotAllowedSetCommentsFiles)
			}
		}

		if err := hlFile.Ffo.FlatFileInformationFork.SetComment(t.GetField(hotline.FieldFileComment).Data); err != nil {
			cc.Logger.Error("set file info: set comment", "err", err)
			return cc.NewErrReply(t, ErrMsgSetFileInfo)
		}
		w, err := hlFile.InfoForkWriter()
		if err != nil {
			cc.Logger.Error("set file info: open info fork writer", "err", err)
			return cc.NewErrReply(t, ErrMsgSetFileInfo)
		}
		_, err = io.Copy(w, &hlFile.Ffo.FlatFileInformationFork)
		if err != nil {
			cc.Logger.Error("set file info: write info fork", "err", err)
			return cc.NewErrReply(t, ErrMsgSetFileInfo)
		}
	}

	fullNewFilePath, err := hotline.ReadPath(cc.FileRoot(), filePath, t.GetField(hotline.FieldFileNewName).Data, cc.TextDecoder())
	if err != nil {
		cc.Logger.Error("set file info: read new name path", "err", err)
		return cc.NewErrReply(t, ErrMsgRenameFile)
	}

	fileNewName := t.GetField(hotline.FieldFileNewName).Data

	if fileNewName != nil {
		switch mode := fi.Mode(); {
		case mode.IsDir():
			if !cc.Authorize(hotline.AccessRenameFolder) {
				return cc.NewErrReply(t, ErrMsgNotAllowedRenameFolders)
			}
			err = cc.Server.FS.Rename(fullFilePath, fullNewFilePath)
			if os.IsNotExist(err) {
				return cc.NewErrReply(t, fmt.Sprintf(ErrMsgCannotRenameFolderNotFound, string(fileName)))
			}
			if err != nil {
				cc.Logger.Error("set file info: rename folder", "err", err)
				return cc.NewErrReply(t, ErrMsgRenameFolder)
			}
		case mode.IsRegular():
			if !cc.Authorize(hotline.AccessRenameFile) {
				return cc.NewErrReply(t, ErrMsgNotAllowedRenameFiles)
			}
			fileDir, err := hotline.ReadPath(cc.FileRoot(), filePath, []byte{}, cc.TextDecoder())
			if err != nil {
				cc.Logger.Error("set file info: read file dir", "err", err)
				return cc.NewErrReply(t, ErrMsgRenameFile)
			}
			hlFile.Name, err = cc.TextDecoder().String(string(fileNewName))
			if err != nil {
				cc.Logger.Error("set file info: decode new name", "err", err)
				return cc.NewErrReply(t, ErrMsgRenameFile)
			}

			err = hlFile.Move(fileDir)
			if os.IsNotExist(err) {
				return cc.NewErrReply(t, fmt.Sprintf(ErrMsgCannotRenameFileNotFound, string(fileName)))
			}
			if err != nil {
				cc.Logger.Error("set file info: rename file", "err", err)
				return cc.NewErrReply(t, ErrMsgRenameFile)
			}
		}
	}

	res = append(res, cc.NewReply(t))
	return res
}

// HandleDeleteFile deletes the specified file from the server.
//
// Access: Delete File (0) or Delete Folder (6)
//
// Fields used in the request:
//   - 201 File name  Required
//   - 202 File path  Required
//
// Fields used in the reply: None
func HandleDeleteFile(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	fileName := t.GetField(hotline.FieldFileName).Data
	filePath := t.GetField(hotline.FieldFilePath).Data

	fullFilePath, err := hotline.ReadPath(cc.FileRoot(), filePath, fileName, cc.TextDecoder())
	if err != nil {
		cc.Logger.Error("delete file: read path", "err", err)
		return cc.NewErrReply(t, ErrMsgFileNotFound)
	}

	hlFile, err := hotline.NewFile(cc.Server.FS, fullFilePath, 0)
	if err != nil {
		cc.Logger.Error("delete file: open file", "err", err)
		return cc.NewErrReply(t, ErrMsgFileNotFound)
	}

	fi, err := hlFile.DataFile()
	if err != nil {
		return cc.NewErrReply(t, fmt.Sprintf(ErrMsgCannotDeleteFileNotFound, string(fileName)))
	}

	switch mode := fi.Mode(); {
	case mode.IsDir():
		if !cc.Authorize(hotline.AccessDeleteFolder) {
			return cc.NewErrReply(t, ErrMsgNotAllowedDeleteFolders)
		}
	case mode.IsRegular():
		if !cc.Authorize(hotline.AccessDeleteFile) {
			return cc.NewErrReply(t, ErrMsgNotAllowedDeleteFiles)
		}
	}

	if err := hlFile.Delete(); err != nil {
		cc.Logger.Error("delete file: delete", "err", err)
		return cc.NewErrReply(t, ErrMsgDeleteFile)
	}

	res = append(res, cc.NewReply(t))
	return res
}

// HandleMoveFile moves a file from one folder to another on the same server.
//
// Fields used in the request:
//   - 201 File name      Required
//   - 202 File path      Required
//   - 212 File new path  Required - Destination path
//
// Fields used in the reply: None
func HandleMoveFile(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	fileName := string(t.GetField(hotline.FieldFileName).Data)

	filePath, err := hotline.ReadPath(cc.FileRoot(), t.GetField(hotline.FieldFilePath).Data, t.GetField(hotline.FieldFileName).Data, cc.TextDecoder())
	if err != nil {
		cc.Logger.Error("move file: read source path", "err", err)
		return cc.NewErrReply(t, ErrMsgFileNotFound)
	}

	fileNewPath, err := hotline.ReadPath(cc.FileRoot(), t.GetField(hotline.FieldFileNewPath).Data, nil, cc.TextDecoder())
	if err != nil {
		cc.Logger.Error("move file: read destination path", "err", err)
		return cc.NewErrReply(t, ErrMsgFileNotFound)
	}

	cc.Logger.Info("Move file", "src", filePath+"/"+fileName, "dst", fileNewPath+"/"+fileName)

	hlFile, err := hotline.NewFile(cc.Server.FS, filePath, 0)
	if err != nil {
		cc.Logger.Error("move file: open file", "err", err)
		return cc.NewErrReply(t, ErrMsgFileNotFound)
	}

	fi, err := hlFile.DataFile()
	if err != nil {
		return cc.NewErrReply(t, fmt.Sprintf(ErrMsgCannotDeleteFileNotFound, fileName))
	}
	switch mode := fi.Mode(); {
	case mode.IsDir():
		if !cc.Authorize(hotline.AccessMoveFolder) {
			return cc.NewErrReply(t, ErrMsgNotAllowedMoveFolders)
		}
	case mode.IsRegular():
		if !cc.Authorize(hotline.AccessMoveFile) {
			return cc.NewErrReply(t, ErrMsgNotAllowedMoveFiles)
		}
	}
	if err := hlFile.Move(fileNewPath); err != nil {
		cc.Logger.Error("move file: move", "err", err)
		return cc.NewErrReply(t, ErrMsgMoveFile)
	}
	// TODO: handle other possible errors; e.g. file delete fails due to permission issue

	res = append(res, cc.NewReply(t))
	return res
}

// HandleNewFolder creates a new folder on the server.
//
// Access: Create Folder (5)
//
// Fields used in the request:
//   - 201 File name  Required
//   - 202 File path  Optional
//
// Fields used in the reply: None
func HandleNewFolder(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessCreateFolder) {
		return cc.NewErrReply(t, ErrMsgNotAllowedCreateFolders)
	}
	folderName := string(t.GetField(hotline.FieldFileName).Data)

	folderName = path.Join("/", folderName)

	var subPath string

	// FieldFilePath is only present for nested paths
	if t.GetField(hotline.FieldFilePath).Data != nil {
		var newFp hotline.FilePath
		_, err := newFp.Write(t.GetField(hotline.FieldFilePath).Data)
		if err != nil {
			cc.Logger.Error("new folder: parse file path", "err", err)
			return cc.NewErrReply(t, ErrMsgCreateFolder)
		}

		for _, pathItem := range newFp.Items {
			subPath = path.Join("/", subPath, string(pathItem.Name))
		}
	}

	// Decode only client-provided path components from Mac Roman to UTF-8.
	// The FileRoot is already a UTF-8 filesystem path and must not be decoded.
	subPath, err := cc.TextDecoder().String(subPath)
	if err != nil {
		cc.Logger.Error("new folder: decode sub path", "err", err)
		return cc.NewErrReply(t, ErrMsgCreateFolder)
	}
	folderName, err = cc.TextDecoder().String(folderName)
	if err != nil {
		cc.Logger.Error("new folder: decode folder name", "err", err)
		return cc.NewErrReply(t, ErrMsgCreateFolder)
	}

	newFolderPath := path.Join(cc.FileRoot(), subPath, folderName)

	// TODO: check path and folder Name lengths

	if _, err := cc.Server.FS.Stat(newFolderPath); !os.IsNotExist(err) {
		msg := fmt.Sprintf(ErrMsgFolderCreateConflictTemplate, folderName)
		return cc.NewErrReply(t, msg)
	}

	if err := cc.Server.FS.Mkdir(newFolderPath, 0777); err != nil {
		msg := fmt.Sprintf(ErrMsgFolderCreateErrorTemplate, folderName)
		return cc.NewErrReply(t, msg)
	}

	return append(res, cc.NewReply(t))
}

// HandleGetFileNameList gets the list of file names from the specified folder.
//
// Fields used in the request:
//   - 202 File path  Optional - If not specified, root folder assumed
//
// Fields used in the reply:
//   - 200 File name with info  Repeated - File information for each item
func HandleGetFileNameList(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	fullPath, err := hotline.ReadPath(
		cc.FileRoot(),
		t.GetField(hotline.FieldFilePath).Data,
		nil,
		cc.TextDecoder(),
	)
	if err != nil {
		cc.Logger.Error("error reading file path", "err", err)
		return cc.NewErrReply(t, "Cannot get file list.")
	}

	var fp hotline.FilePath
	if t.GetField(hotline.FieldFilePath).Data != nil {
		if _, err = fp.Write(t.GetField(hotline.FieldFilePath).Data); err != nil {
			cc.Logger.Error("error parsing file path", "err", err)
			return cc.NewErrReply(t, "Cannot get file list.")
		}
	}

	// Handle special case for drop box folders
	if fp.IsDropbox() && !cc.Authorize(hotline.AccessViewDropBoxes) {
		return cc.NewErrReply(t, ErrMsgNotAllowedViewDropBoxes)
	}

	fileNames, err := hotline.GetFileNameList(cc.Server.FS, fullPath, cc.Server.Config.IgnoreFiles, cc.TextEncoder(), cc.Logger)
	if err != nil {
		cc.Logger.Error("error getting file name list", "err", err)
		return cc.NewErrReply(t, "Cannot get file list.")
	}

	res = append(res, cc.NewReply(t, fileNames...))

	return res
}

// =================================
//     Hotline private chat flow
// =================================
// 1. ClientA sends TranInviteNewChat to server with user Type to invite
// 2. Server creates new ChatID
// 3. Server sends TranInviteToChat to invitee
// 4. Server replies to ClientA with new Chat Type
//
// A dialog box pops up in the invitee client with options to accept or decline the invitation.
// If Accepted is clicked:
// 1. ClientB sends TranJoinChat with FieldChatID

// HandleMakeAlias makes a file alias using the specified path.
//
// Access: Make Alias (31)
//
// Fields used in the request:
//   - 201 File name      Required
//   - 202 File path      Required
//   - 212 File new path  Required - Destination path
//
// Fields used in the reply: None
func HandleMakeAlias(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessMakeAlias) {
		return cc.NewErrReply(t, ErrMsgNotAllowedMakeAliases)
	}
	fileName := t.GetField(hotline.FieldFileName).Data
	filePath := t.GetField(hotline.FieldFilePath).Data
	fileNewPath := t.GetField(hotline.FieldFileNewPath).Data

	fullFilePath, err := hotline.ReadPath(cc.FileRoot(), filePath, fileName, cc.TextDecoder())
	if err != nil {
		cc.Logger.Error("make alias: read source path", "err", err)
		return cc.NewErrReply(t, ErrMsgFileNotFound)
	}

	fullNewFilePath, err := hotline.ReadPath(cc.FileRoot(), fileNewPath, fileName, cc.TextDecoder())
	if err != nil {
		cc.Logger.Error("make alias: read destination path", "err", err)
		return cc.NewErrReply(t, ErrMsgFileNotFound)
	}

	if err := cc.Server.FS.Symlink(fullFilePath, fullNewFilePath); err != nil {
		return cc.NewErrReply(t, ErrMsgCreateAlias)
	}

	res = append(res, cc.NewReply(t))
	return res
}
