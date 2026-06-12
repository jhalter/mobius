package mobius

import (
	"encoding/binary"
	"fmt"

	"github.com/jhalter/mobius/hotline"
)

// HandleDownloadFile downloads a file from the specified path on the server.
//
// Access: Download File (2)
//
// Fields used in the request:
//   - 201 File name              Required
//   - 202 File path              Optional
//   - 203 File resume data       Optional
//   - 204 File transfer options  Optional - Set to 2 for TEXT, JPEG, GIFF, BMP or PICT files
//
// Fields used in the reply:
//   - 108 Transfer size     Size of data to be downloaded
//   - 207 File size         Actual file size
//   - 107 Reference number  Used later for transfer
//   - 116 Waiting count     Queue position
func HandleDownloadFile(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessDownloadFile) {
		return cc.NewErrReply(t, ErrMsgNotAllowedDownloadFiles)
	}

	fileName := t.GetField(hotline.FieldFileName).Data
	filePath := t.GetField(hotline.FieldFilePath).Data
	resumeData := t.GetField(hotline.FieldFileResumeData).Data

	var dataOffset int64
	var frd hotline.FileResumeData
	if resumeData != nil {
		if err := frd.UnmarshalBinary(t.GetField(hotline.FieldFileResumeData).Data); err != nil {
			cc.Logger.Error("download file: unmarshal resume data", "err", err)
			return cc.NewErrReply(t, ErrMsgFileResumeData)
		}
		// TODO: handle rsrc fork offset
		if len(frd.ForkInfoList) > 0 {
			dataOffset = int64(binary.BigEndian.Uint32(frd.ForkInfoList[0].DataSize[:]))
		}
	}

	fullFilePath, err := hotline.ReadPath(cc.FileRoot(), filePath, fileName, cc.TextDecoder())
	if err != nil {
		cc.Logger.Error("download file: read path", "err", err)
		return cc.NewErrReply(t, ErrMsgFileNotFound)
	}

	hlFile, err := hotline.NewFile(cc.Server.FS, fullFilePath, dataOffset)
	if err != nil {
		cc.Logger.Error("download file: open file", "err", err)
		return cc.NewErrReply(t, ErrMsgFileNotFound)
	}

	xferSize := hlFile.Ffo.TransferSize(0)

	ft := cc.NewFileTransfer(
		hotline.FileDownload,
		cc.FileRoot(),
		fileName,
		filePath,
		xferSize,
	)

	if resumeData != nil {
		var frd hotline.FileResumeData
		if err := frd.UnmarshalBinary(t.GetField(hotline.FieldFileResumeData).Data); err != nil {
			cc.Logger.Error("download file: unmarshal resume data", "err", err)
			return cc.NewErrReply(t, ErrMsgFileResumeData)
		}
		ft.FileResumeData = &frd
	}

	// Optional field for when a client requests file preview
	// Used only for TEXT, JPEG, GIFF, BMP or PICT files
	// The value will always be 2
	if t.GetField(hotline.FieldFileTransferOptions).Data != nil {
		ft.Options = t.GetField(hotline.FieldFileTransferOptions).Data
		xferSize = hlFile.Ffo.FlatFileDataForkHeader.DataSize[:]
	}

	res = append(res, cc.NewReply(t,
		hotline.NewField(hotline.FieldRefNum, ft.RefNum[:]),
		hotline.NewField(hotline.FieldWaitingCount, []byte{0x00, 0x00}), // TODO: Implement waiting count
		hotline.NewField(hotline.FieldTransferSize, xferSize),
		hotline.NewField(hotline.FieldFileSize, hlFile.Ffo.FlatFileDataForkHeader.DataSize[:]),
	))

	return res
}

// HandleDownloadFolder downloads all files from the specified folder and its subfolders.
//
// Access: Download File (2)
//
// Fields used in the request:
//   - 201 File name  Required
//   - 202 File path  Optional
//
// Fields used in the reply:
//   - 220 Folder item count  Number of items in the folder
//   - 107 Reference number   Used later for transfer
//   - 108 Transfer size      Size of data to be downloaded
//   - 116 Waiting count      Queue position
func HandleDownloadFolder(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessDownloadFolder) {
		return cc.NewErrReply(t, ErrMsgNotAllowedDownloadFolders)
	}

	fullFilePath, err := hotline.ReadPath(cc.FileRoot(), t.GetField(hotline.FieldFilePath).Data, t.GetField(hotline.FieldFileName).Data, cc.TextDecoder())
	if err != nil {
		cc.Logger.Error("download folder: read path", "err", err)
		return cc.NewErrReply(t, ErrMsgFileNotFound)
	}

	transferSize, err := hotline.CalcTotalSize(fullFilePath)
	if err != nil {
		cc.Logger.Error("download folder: calc total size", "err", err)
		return cc.NewErrReply(t, ErrMsgDownloadFolder)
	}
	itemCount, err := hotline.CalcItemCount(fullFilePath)
	if err != nil {
		cc.Logger.Error("download folder: calc item count", "err", err)
		return cc.NewErrReply(t, ErrMsgDownloadFolder)
	}

	fileTransfer := cc.NewFileTransfer(hotline.FolderDownload, cc.FileRoot(), t.GetField(hotline.FieldFileName).Data, t.GetField(hotline.FieldFilePath).Data, transferSize)

	var fp hotline.FilePath
	_, err = fp.Write(t.GetField(hotline.FieldFilePath).Data)
	if err != nil {
		cc.Logger.Error("download folder: parse file path", "err", err)
		return cc.NewErrReply(t, ErrMsgDownloadFolder)
	}

	res = append(res, cc.NewReply(t,
		hotline.NewField(hotline.FieldRefNum, fileTransfer.RefNum[:]),
		hotline.NewField(hotline.FieldTransferSize, transferSize),
		hotline.NewField(hotline.FieldFolderItemCount, itemCount),
		hotline.NewField(hotline.FieldWaitingCount, []byte{0x00, 0x00}), // TODO: Implement waiting count
	))
	return res
}

// HandleUploadFolder uploads all files from a local folder and its subfolders to the server.
//
// Access: Upload File (1)
//
// Fields used in the request:
//   - 201 File name              Required
//   - 202 File path              Optional
//   - 108 Transfer size          Total size of all items in the folder
//   - 220 Folder item count      Number of items in the folder
//   - 204 File transfer options  Optional - Currently set to 1
//
// Fields used in the reply:
//   - 107 Reference number  Used later for transfer
func HandleUploadFolder(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessUploadFolder) {
		return cc.NewErrReply(t, ErrMsgNotAllowedUploadFolders)
	}

	var fp hotline.FilePath
	if t.GetField(hotline.FieldFilePath).Data != nil {
		if _, err := fp.Write(t.GetField(hotline.FieldFilePath).Data); err != nil {
			cc.Logger.Error("upload folder: parse file path", "err", err)
			return cc.NewErrReply(t, ErrMsgUploadFolder)
		}
	}

	// Handle special cases for Upload and Drop Box folders
	if !cc.Authorize(hotline.AccessUploadAnywhere) {
		if !fp.IsUploadDir() && !fp.IsDropbox() {
			return cc.NewErrReply(t, fmt.Sprintf(ErrMsgUploadRestrictedTemplate, "folder", string(t.GetField(hotline.FieldFileName).Data)))
		}
	}

	fileTransfer := cc.NewFileTransfer(hotline.FolderUpload,
		cc.FileRoot(),
		t.GetField(hotline.FieldFileName).Data,
		t.GetField(hotline.FieldFilePath).Data,
		t.GetField(hotline.FieldTransferSize).Data,
	)

	fileTransfer.FolderItemCount = t.GetField(hotline.FieldFolderItemCount).Data

	return append(res, cc.NewReply(t, hotline.NewField(hotline.FieldRefNum, fileTransfer.RefNum[:])))
}

// HandleUploadFile uploads a file to the specified path on the server.
//
// Access: Upload File (1)
//
// Fields used in the request:
//   - 201 File name              Required
//   - 202 File path              Optional
//   - 204 File transfer options  Optional - Used to resume download, value 2
//   - 108 File transfer size     Optional - Used if download is not resumed
//
// Fields used in the reply:
//   - 203 File resume data  Optional - Used only to resume download
//   - 107 Reference number  Transfer reference
func HandleUploadFile(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	if !cc.Authorize(hotline.AccessUploadFile) {
		return cc.NewErrReply(t, ErrMsgNotAllowedUploadFiles)
	}

	fileName := t.GetField(hotline.FieldFileName).Data
	filePath := t.GetField(hotline.FieldFilePath).Data
	transferOptions := t.GetField(hotline.FieldFileTransferOptions).Data
	transferSize := t.GetField(hotline.FieldTransferSize).Data // not sent for resume

	var fp hotline.FilePath
	if filePath != nil {
		if _, err := fp.Write(filePath); err != nil {
			cc.Logger.Error("upload file: parse file path", "err", err)
			return cc.NewErrReply(t, ErrMsgUploadFile)
		}
	}

	// Handle special cases for Upload and Drop Box folders
	if !cc.Authorize(hotline.AccessUploadAnywhere) {
		if !fp.IsUploadDir() && !fp.IsDropbox() {
			return cc.NewErrReply(t, fmt.Sprintf(ErrMsgUploadRestrictedTemplate, "file", string(fileName)))
		}
	}
	fullFilePath, err := hotline.ReadPath(cc.FileRoot(), filePath, fileName, cc.TextDecoder())
	if err != nil {
		cc.Logger.Error("upload file: read path", "err", err)
		return cc.NewErrReply(t, ErrMsgUploadFile)
	}

	if _, err := cc.Server.FS.Stat(fullFilePath); err == nil {
		return cc.NewErrReply(t, fmt.Sprintf(ErrMsgFileUploadConflictTemplate, string(fileName)))
	}

	ft := cc.NewFileTransfer(hotline.FileUpload, cc.FileRoot(), fileName, filePath, transferSize)

	replyT := cc.NewReply(t, hotline.NewField(hotline.FieldRefNum, ft.RefNum[:]))

	// client has requested to resume a partially transferred file
	if transferOptions != nil {
		// If there is no partial file to resume from, fall back to a normal upload
		// reply (with the reference number already set) rather than discarding it.
		if fileInfo, err := cc.Server.FS.Stat(fullFilePath + hotline.IncompleteFileSuffix); err != nil {
			cc.Logger.Info("upload file: no partial file to resume, starting fresh upload", "err", err)
		} else {
			offset := make([]byte, 4)
			binary.BigEndian.PutUint32(offset, uint32(fileInfo.Size()))

			fileResumeData := hotline.NewFileResumeData([]hotline.ForkInfoList{
				*hotline.NewForkInfoList(offset),
			})

			b, _ := fileResumeData.BinaryMarshal()

			ft.TransferSize = offset

			replyT.Fields = append(replyT.Fields, hotline.NewField(hotline.FieldFileResumeData, b))
		}
	}

	res = append(res, replyT)
	return res
}

// HandleDownloadBanner requests a new banner from the server.
//
// Fields used in the request: None
//
// Fields used in the reply:
//   - 107 Reference number  Used later for transfer
//   - 108 Transfer size     Size of data to be downloaded
func HandleDownloadBanner(cc *hotline.ClientConn, t *hotline.Transaction) (res []hotline.Transaction) {
	ft := cc.NewFileTransfer(hotline.BannerDownload, "", []byte{}, []byte{}, make([]byte, 4))
	binary.BigEndian.PutUint32(ft.TransferSize, uint32(len(cc.Server.Banner)))

	return append(res, cc.NewReply(t,
		hotline.NewField(hotline.FieldRefNum, ft.RefNum[:]),
		hotline.NewField(hotline.FieldTransferSize, ft.TransferSize),
	))
}
