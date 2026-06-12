package mobius

// Public error message constants for reuse by other packages
const (
	// Authorization error messages
	ErrMsgNotAllowedParticipateChat      = "You are not allowed to participate in chat."
	ErrMsgNotAllowedSendPrivateMsg       = "You are not allowed to send private messages."
	ErrMsgNotAllowedReadNews             = "You are not allowed to read news."
	ErrMsgNotAllowedPostNews             = "You are not allowed to post news."
	ErrMsgNotAllowedCreateAccounts       = "You are not allowed to create new accounts."
	ErrMsgNotAllowedViewAccounts         = "You are not allowed to view accounts."
	ErrMsgNotAllowedModifyAccounts       = "You are not allowed to modify accounts."
	ErrMsgNotAllowedDeleteAccounts       = "You are not allowed to delete accounts."
	ErrMsgNotAllowedRequestPrivateChat   = "You are not allowed to request private chat."
	ErrMsgNotAllowedCreateNewsCategories = "You are not allowed to create news categories."
	ErrMsgNotAllowedDeleteNewsArticles   = "You are not allowed to delete news articles."
	ErrMsgNotAllowedSetCommentsFiles     = "You are not allowed to set comments for files."
	ErrMsgNotAllowedSetCommentsFolders   = "You are not allowed to set comments for folders."
	ErrMsgNotAllowedRenameFiles          = "You are not allowed to rename files."
	ErrMsgNotAllowedRenameFolders        = "You are not allowed to rename folders."
	ErrMsgNotAllowedDeleteFiles          = "You are not allowed to delete files."
	ErrMsgNotAllowedDeleteFolders        = "You are not allowed to delete folders."
	ErrMsgNotAllowedMoveFiles            = "You are not allowed to move files."
	ErrMsgNotAllowedMoveFolders          = "You are not allowed to move folders."
	ErrMsgNotAllowedCreateFolders        = "You are not allowed to create folders."
	ErrMsgNotAllowedSendBroadcast        = "You are not allowed to send broadcast messages."
	ErrMsgNotAllowedGetClientInfo        = "You are not allowed to get client info."
	ErrMsgNotAllowedDisconnectUsers      = "You are not allowed to disconnect users."
	ErrMsgNotAllowedCreateNewsfolders    = "You are not allowed to create news folders."
	ErrMsgNotAllowedDeleteNewsCategories = "You are not allowed to delete news categories."
	ErrMsgNotAllowedDeleteNewsFolders    = "You are not allowed to delete news folders."
	ErrMsgNotAllowedPostNewsArticles     = "You are not allowed to post news articles."
	ErrMsgNotAllowedDownloadFiles        = "You are not allowed to download files."
	ErrMsgNotAllowedDownloadFolders      = "You are not allowed to download folders."
	ErrMsgNotAllowedUploadFiles          = "You are not allowed to upload files."
	ErrMsgNotAllowedUploadFolders        = "You are not allowed to upload folders."
	ErrMsgNotAllowedViewDropBoxes        = "You are not allowed to view drop boxes."
	ErrMsgNotAllowedMakeAliases          = "You are not allowed to make aliases."

	// Account error messages
	ErrMsgAccountDeleted    = "You are logged in with an account which was deleted."
	ErrMsgAccountExists     = "Cannot create account because there is already an account with that login."
	ErrMsgAccountMoreAccess = "Cannot create account with more access than yourself."
	ErrMsgAccountNotExist   = "Account does not exist."

	// Account error templates (for dynamic content)
	ErrMsgAccountExistsTemplate = "Cannot create account %s because there is already an account with that login."

	// File operation error templates
	ErrMsgCannotRenameFileNotFound   = "Cannot rename file %s because it does not exist or cannot be found."
	ErrMsgCannotRenameFolderNotFound = "Cannot rename folder %s because it does not exist or cannot be found."
	ErrMsgCannotDeleteFileNotFound   = "Cannot delete file %s because it does not exist or cannot be found."

	// File operation error templates (for dynamic content)
	ErrMsgFolderCreateConflictTemplate = "Cannot create folder \"%s\" because there is already a file or folder with that Name."
	ErrMsgFolderCreateErrorTemplate    = "Cannot create folder \"%s\" because an error occurred."

	// Upload restriction templates (these need dynamic content)
	ErrMsgUploadRestrictedTemplate   = "Cannot accept upload of the %s \"%v\" because you are only allowed to upload to the \"Uploads\" folder."
	ErrMsgFileUploadConflictTemplate = "Cannot accept upload because there is already a file named \"%v\". Try choosing a different Name."

	// Chat/messaging templates (these need dynamic content)
	ErrMsgDoesNotAcceptTemplate = "%s does not accept %s."

	// Ban messages
	ErrMsgTemporaryBan = "You are temporarily banned on this server"
	ErrMsgPermanentBan = "You are permanently banned on this server"

	// General error messages
	ErrMsgFileNotFound       = "File not found."
	ErrMsgGetFileInfo        = "Error getting file information."
	ErrMsgSetFileInfo        = "Error setting file information."
	ErrMsgRenameFile         = "Error renaming file."
	ErrMsgRenameFolder       = "Error renaming folder."
	ErrMsgDeleteFile         = "Error deleting file."
	ErrMsgMoveFile           = "Error moving file."
	ErrMsgCreateFolder       = "Error creating folder."
	ErrMsgDownloadFolder     = "Error downloading folder."
	ErrMsgUploadFile         = "Error uploading file."
	ErrMsgUploadFolder       = "Error uploading folder."
	ErrMsgFileResumeData     = "Invalid file resume data."
	ErrMsgAccountNotFound    = "Account not found."
	ErrMsgUserNotFound       = "User not found."
	ErrMsgInvalidUserID      = "Invalid user ID."
	ErrMsgInvalidChatID      = "Invalid chat ID."
	ErrMsgCreateAlias        = "Error creating alias"
	ErrMsgUpdateAccount      = "Error updating account."
	ErrMsgDeleteAccount      = "Error deleting account."
	ErrMsgGetUserList        = "Error getting user list."
	ErrMsgJoinChat           = "Error joining chat."
	ErrMsgReadNewsCategories = "Error reading news categories."
	ErrMsgReadNewsArticles   = "Error reading news articles."
	ErrMsgCreateNewsCategory = "Error creating news category."
	ErrMsgCreateNewsFolder   = "Error creating news folder."
	ErrMsgDeleteNewsArticle  = "Error deleting news article."
	ErrMsgDeleteNewsItem     = "Error deleting news item."
	ErrMsgPostNewsArticle    = "Error posting news article."
	ErrMsgPostNews           = "Error posting news."
	ErrMsgReadMessageBoard   = "Error reading message board."
)
