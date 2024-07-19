package hotline

import (
	"bufio"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// Folder download actions.  Send by the client to indicate the next action the server should take
// for a folder download.
const (
	DlFldrActionSendFile   = 1
	DlFldrActionResumeFile = 2
	DlFldrActionNextFile   = 3
)

// File transfer types
type FileTransferType uint8

const (
	FileDownload   = FileTransferType(0)
	FileUpload     = FileTransferType(1)
	FolderDownload = FileTransferType(2)
	FolderUpload   = FileTransferType(3)
	BannerDownload = FileTransferType(4)
)

type FileTransferID [4]byte

type FileTransferMgr interface {
	Add(ft *FileTransfer)
	Get(id FileTransferID) *FileTransfer
	Delete(id FileTransferID)
}

type MemFileTransferMgr struct {
	fileTransfers map[FileTransferID]*FileTransfer

	mu sync.Mutex
}

func NewMemFileTransferMgr() *MemFileTransferMgr {
	return &MemFileTransferMgr{
		fileTransfers: make(map[FileTransferID]*FileTransfer),
	}
}

func (ftm *MemFileTransferMgr) Add(ft *FileTransfer) {
	ftm.mu.Lock()
	defer ftm.mu.Unlock()

	_, _ = rand.Read(ft.RefNum[:])

	ftm.fileTransfers[ft.RefNum] = ft

	ft.ClientConn.ClientFileTransferMgr.Add(ft.Type, ft)
}

func (ftm *MemFileTransferMgr) Get(id FileTransferID) *FileTransfer {
	ftm.mu.Lock()
	defer ftm.mu.Unlock()

	return ftm.fileTransfers[id]
}

func (ftm *MemFileTransferMgr) Delete(id FileTransferID) {
	ftm.mu.Lock()
	defer ftm.mu.Unlock()

	ft := ftm.fileTransfers[id]

	ft.ClientConn.ClientFileTransferMgr.Delete(ft.Type, id)

	delete(ftm.fileTransfers, id)

}

type FileTransfer struct {
	FileName         []byte
	FilePath         []byte
	RefNum           [4]byte
	Type             FileTransferType
	TransferSize     []byte
	FolderItemCount  []byte
	FileResumeData   *FileResumeData
	Options          []byte
	bytesSentCounter *WriteCounter
	ClientConn       *ClientConn
}

// WriteCounter counts the number of bytes written to it.
type WriteCounter struct {
	mux   sync.Mutex
	Total int64 // Total # of bytes written
}

// Write implements the io.Writer interface.
//
// Always completes and never returns an error.
func (wc *WriteCounter) Write(p []byte) (int, error) {
	wc.mux.Lock()
	defer wc.mux.Unlock()
	n := len(p)
	wc.Total += int64(n)
	return n, nil
}

func (cc *ClientConn) NewFileTransfer(transferType FileTransferType, fileName, filePath, size []byte) *FileTransfer {
	ft := &FileTransfer{
		FileName:         fileName,
		FilePath:         filePath,
		Type:             transferType,
		TransferSize:     size,
		ClientConn:       cc,
		bytesSentCounter: &WriteCounter{},
	}

	cc.Server.FileTransferMgr.Add(ft)

	return ft
}

// String returns a string representation of a file transfer and its progress for display in the GetInfo window
// Example:
// MasterOfOrionII1.4.0. 0%   197.9M
func (ft *FileTransfer) String() string {
	trunc := fmt.Sprintf("%.21s", ft.FileName)
	return fmt.Sprintf("%-21s %.3s%%  %6s\n", trunc, ft.percentComplete(), ft.formattedTransferSize())
}

func (ft *FileTransfer) percentComplete() string {
	ft.bytesSentCounter.mux.Lock()
	defer ft.bytesSentCounter.mux.Unlock()
	return fmt.Sprintf(
		"%v",
		math.RoundToEven(float64(ft.bytesSentCounter.Total)/float64(binary.BigEndian.Uint32(ft.TransferSize))*100),
	)
}

func (ft *FileTransfer) formattedTransferSize() string {
	sizeInKB := float32(binary.BigEndian.Uint32(ft.TransferSize)) / 1024
	if sizeInKB >= 1024 {
		return fmt.Sprintf("%.1fM", sizeInKB/1024)
	} else {
		return fmt.Sprintf("%.0fK", sizeInKB)
	}
}

func (ft *FileTransfer) ItemCount() int {
	return int(binary.BigEndian.Uint16(ft.FolderItemCount))
}

type folderUpload struct {
	DataSize      [2]byte
	IsFolder      [2]byte
	PathItemCount [2]byte
	FileNamePath  []byte
}

//func (fu *folderUpload) Write(p []byte) (int, error) {
//	if len(p) < 7 {
//		return 0, errors.New("buflen too short")
//	}
//	copy(fu.DataSize[:], p[0:2])
//	copy(fu.IsFolder[:], p[2:4])
//	copy(fu.PathItemCount[:], p[4:6])
//
//	fu.FileNamePath = make([]byte, binary.BigEndian.Uint16(fu.DataSize[:])-4) // -4 to subtract the path separator bytes TODO: wat
//	n, err := io.ReadFull(rwc, fu.FileNamePath)
//	if err != nil {
//		return 0, err
//	}
//
//	return n + 6, nil
//}

func (fu *folderUpload) FormattedPath() string {
	pathItemLen := binary.BigEndian.Uint16(fu.PathItemCount[:])

	var pathSegments []string
	pathData := fu.FileNamePath

	// TODO: implement scanner interface instead?
	for i := uint16(0); i < pathItemLen; i++ {
		segLen := pathData[2]
		pathSegments = append(pathSegments, string(pathData[3:3+segLen]))
		pathData = pathData[3+segLen:]
	}

	return filepath.Join(pathSegments...)
}

type FileHeader struct {
	Size     [2]byte // Total size of FileHeader payload
	Type     [2]byte // 0 for file, 1 for dir
	FilePath []byte  // encoded file path

	readOffset int // Internal offset to track read progress
}

func NewFileHeader(fileName string, isDir bool) FileHeader {
	fh := FileHeader{
		FilePath: EncodeFilePath(fileName),
	}
	if isDir {
		fh.Type = [2]byte{0x00, 0x01}
	}

	encodedPathLen := uint16(len(fh.FilePath) + len(fh.Type))
	binary.BigEndian.PutUint16(fh.Size[:], encodedPathLen)

	return fh
}

func (fh *FileHeader) Read(p []byte) (int, error) {
	buf := slices.Concat(
		fh.Size[:],
		fh.Type[:],
		fh.FilePath,
	)

	if fh.readOffset >= len(buf) {
		return 0, io.EOF // All bytes have been read
	}

	n := copy(p, buf[fh.readOffset:])
	fh.readOffset += n

	return n, nil
}

func DownloadHandler(w io.Writer, fullPath string, fileTransfer *FileTransfer, fs FileStore, rLogger *slog.Logger, preserveForks bool) error {
	//s.Stats.DownloadCounter += 1
	//s.Stats.DownloadsInProgress += 1
	//defer func() {
	//	s.Stats.DownloadsInProgress -= 1
	//}()

	var dataOffset int64
	if fileTransfer.FileResumeData != nil {
		dataOffset = int64(binary.BigEndian.Uint32(fileTransfer.FileResumeData.ForkInfoList[0].DataSize[:]))
	}

	fw, err := NewFileWrapper(fs, fullPath, 0)
	if err != nil {
		return fmt.Errorf("reading file header: %v", err)
	}

	rLogger.Info("Download file", "filePath", fullPath)

	// If file transfer options are included, that means this is a "quick preview" request.  In this case skip sending
	// the flat file info and proceed directly to sending the file data.
	if fileTransfer.Options == nil {
		if _, err = io.Copy(w, fw.Ffo); err != nil {
			return fmt.Errorf("send flat file object: %v", err)
		}
	}

	file, err := fw.dataForkReader()
	if err != nil {
		return fmt.Errorf("open data fork reader: %v", err)
	}

	br := bufio.NewReader(file)
	if _, err := br.Discard(int(dataOffset)); err != nil {
		return fmt.Errorf("seek to resume offsent: %v", err)
	}

	if _, err = io.Copy(w, io.TeeReader(br, fileTransfer.bytesSentCounter)); err != nil {
		return fmt.Errorf("send data fork: %v", err)
	}

	// If the client requested to resume transfer, do not send the resource fork header.
	if fileTransfer.FileResumeData == nil {
		err = binary.Write(w, binary.BigEndian, fw.rsrcForkHeader())
		if err != nil {
			return fmt.Errorf("send resource fork header: %v", err)
		}
	}

	rFile, _ := fw.rsrcForkFile()
	//if err != nil {
	//	// return fmt.Errorf("open resource fork file: %v", err)
	//}

	_, _ = io.Copy(w, io.TeeReader(rFile, fileTransfer.bytesSentCounter))
	//if err != nil {
	//	// return fmt.Errorf("send resource fork data: %v", err)
	//}

	return nil
}

func UploadHandler(rwc io.ReadWriter, fullPath string, fileTransfer *FileTransfer, fileStore FileStore, rLogger *slog.Logger, preserveForks bool) error {
	var file *os.File

	// A file upload has two possible cases:
	// 1) Upload a new file
	// 2) Resume a partially transferred file
	//  We have to infer which case applies by inspecting what is already on the filesystem

	// Check for existing file.  If found, do not proceed.  This is an invalid scenario, as the file upload transaction
	// handler should have returned an error to the client indicating there was an existing file present.
	_, err := os.Stat(fullPath)
	if err == nil {
		return fmt.Errorf("existing file found: %s", fullPath)
	}
	if errors.Is(err, fs.ErrNotExist) {
		// If not found, open or create a new .incomplete file
		file, err = os.OpenFile(fullPath+IncompleteFileSuffix, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
	}

	f, err := NewFileWrapper(fileStore, fullPath, 0)
	if err != nil {
		return err
	}

	rLogger.Info("File upload started", "dstFile", fullPath)

	rForkWriter := io.Discard
	iForkWriter := io.Discard
	if preserveForks {
		rForkWriter, err = f.rsrcForkWriter()
		if err != nil {
			return err
		}

		iForkWriter, err = f.InfoForkWriter()
		if err != nil {
			return err
		}
	}

	if err := receiveFile(rwc, file, rForkWriter, iForkWriter, fileTransfer.bytesSentCounter); err != nil {
		rLogger.Error(err.Error())
	}

	if err := file.Close(); err != nil {
		return err
	}

	if err := fileStore.Rename(fullPath+".incomplete", fullPath); err != nil {
		return err
	}

	rLogger.Info("File upload complete", "dstFile", fullPath)

	return nil
}

func DownloadFolderHandler(rwc io.ReadWriter, fullPath string, fileTransfer *FileTransfer, fileStore FileStore, rLogger *slog.Logger, preserveForks bool) error {
	// Folder Download flow:
	// 1. Get filePath from the transfer
	// 2. Iterate over files
	// 3. For each fileWrapper:
	// 	 Send fileWrapper header to client
	// The client can reply in 3 ways:
	//
	// 1. If type is an odd number (unknown type?), or fileWrapper download for the current fileWrapper is completed:
	//		client sends []byte{0x00, 0x03} to tell the server to continue to the next fileWrapper
	//
	// 2. If download of a fileWrapper is to be resumed:
	//		client sends:
	//			[]byte{0x00, 0x02} // download folder action
	//			[2]byte // Resume data size
	//			[]byte fileWrapper resume data (see myField_FileResumeData)
	//
	// 3. Otherwise, download of the fileWrapper is requested and client sends []byte{0x00, 0x01}
	//
	// When download is requested (case 2 or 3), server replies with:
	// 			[4]byte - fileWrapper size
	//			[]byte  - Flattened File Object
	//
	// After every fileWrapper download, client could request next fileWrapper with:
	// 			[]byte{0x00, 0x03}
	//
	// This notifies the server to send the next item header

	basePathLen := len(fullPath)

	rLogger.Info("Start folder download", "path", fullPath)

	nextAction := make([]byte, 2)
	if _, err := io.ReadFull(rwc, nextAction); err != nil {
		return err
	}

	i := 0
	err := filepath.Walk(fullPath+"/", func(path string, info os.FileInfo, err error) error {
		//s.Stats.DownloadCounter += 1
		i += 1

		if err != nil {
			return err
		}

		// skip dot files
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		hlFile, err := NewFileWrapper(fileStore, path, 0)
		if err != nil {
			return err
		}

		subPath := path[basePathLen+1:]
		rLogger.Debug("Sending fileheader", "i", i, "path", path, "fullFilePath", fullPath, "subPath", subPath, "IsDir", info.IsDir())

		if i == 1 {
			return nil
		}

		fileHeader := NewFileHeader(subPath, info.IsDir())
		if _, err := io.Copy(rwc, &fileHeader); err != nil {
			return fmt.Errorf("error sending file header: %w", err)
		}

		// Read the client's Next Action request
		if _, err := io.ReadFull(rwc, nextAction); err != nil {
			return err
		}

		rLogger.Debug("Client folder download action", "action", fmt.Sprintf("%X", nextAction[0:2]))

		var dataOffset int64

		switch nextAction[1] {
		case DlFldrActionResumeFile:
			// get size of resumeData
			resumeDataByteLen := make([]byte, 2)
			if _, err := io.ReadFull(rwc, resumeDataByteLen); err != nil {
				return err
			}

			resumeDataLen := binary.BigEndian.Uint16(resumeDataByteLen)
			resumeDataBytes := make([]byte, resumeDataLen)
			if _, err := io.ReadFull(rwc, resumeDataBytes); err != nil {
				return err
			}

			var frd FileResumeData
			if err := frd.UnmarshalBinary(resumeDataBytes); err != nil {
				return err
			}
			dataOffset = int64(binary.BigEndian.Uint32(frd.ForkInfoList[0].DataSize[:]))
		case DlFldrActionNextFile:
			// client asked to skip this file
			return nil
		}

		if info.IsDir() {
			return nil
		}

		rLogger.Info("File download started",
			"fileName", info.Name(),
			"TransferSize", fmt.Sprintf("%x", hlFile.Ffo.TransferSize(dataOffset)),
		)

		// Send file size to client
		if _, err := rwc.Write(hlFile.Ffo.TransferSize(dataOffset)); err != nil {
			rLogger.Error(err.Error())
			return fmt.Errorf("error sending file size: %w", err)
		}

		// Send ffo bytes to client
		_, err = io.Copy(rwc, hlFile.Ffo)
		if err != nil {
			return fmt.Errorf("error sending flat file object: %w", err)
		}

		file, err := fileStore.Open(path)
		if err != nil {
			return fmt.Errorf("error opening file: %w", err)
		}

		// wr := bufio.NewWriterSize(rwc, 1460)
		if _, err = io.Copy(rwc, io.TeeReader(file, fileTransfer.bytesSentCounter)); err != nil {
			return fmt.Errorf("error sending file: %w", err)
		}

		if nextAction[1] != 2 && hlFile.Ffo.FlatFileHeader.ForkCount[1] == 3 {
			err = binary.Write(rwc, binary.BigEndian, hlFile.rsrcForkHeader())
			if err != nil {
				return fmt.Errorf("error sending resource fork header: %w", err)
			}

			rFile, err := hlFile.rsrcForkFile()
			if err != nil {
				return fmt.Errorf("error opening resource fork: %w", err)
			}

			if _, err = io.Copy(rwc, io.TeeReader(rFile, fileTransfer.bytesSentCounter)); err != nil {
				return fmt.Errorf("error sending resource fork: %w", err)
			}
		}

		// Read the client's Next Action request.  This is always 3, I think?
		if _, err := io.ReadFull(rwc, nextAction); err != nil && err != io.EOF {
			return fmt.Errorf("error reading client next action: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

func UploadFolderHandler(rwc io.ReadWriter, fullPath string, fileTransfer *FileTransfer, fileStore FileStore, rLogger *slog.Logger, preserveForks bool) error {

	// Check if the target folder exists.  If not, create it.
	if _, err := fileStore.Stat(fullPath); os.IsNotExist(err) {
		if err := fileStore.Mkdir(fullPath, 0777); err != nil {
			return err
		}
	}

	// Begin the folder upload flow by sending the "next file action" to client
	if _, err := rwc.Write([]byte{0, DlFldrActionNextFile}); err != nil {
		return err
	}

	fileSize := make([]byte, 4)

	for i := 0; i < fileTransfer.ItemCount(); i++ {
		//s.Stats.UploadCounter += 1

		var fu folderUpload
		// TODO: implement io.Writer on folderUpload and replace this
		if _, err := io.ReadFull(rwc, fu.DataSize[:]); err != nil {
			return err
		}
		if _, err := io.ReadFull(rwc, fu.IsFolder[:]); err != nil {
			return err
		}
		if _, err := io.ReadFull(rwc, fu.PathItemCount[:]); err != nil {
			return err
		}
		fu.FileNamePath = make([]byte, binary.BigEndian.Uint16(fu.DataSize[:])-4) // -4 to subtract the path separator bytes TODO: wat
		if _, err := io.ReadFull(rwc, fu.FileNamePath); err != nil {
			return err
		}

		if fu.IsFolder == [2]byte{0, 1} {
			if _, err := os.Stat(filepath.Join(fullPath, fu.FormattedPath())); os.IsNotExist(err) {
				if err := os.Mkdir(filepath.Join(fullPath, fu.FormattedPath()), 0777); err != nil {
					return err
				}
			}

			// Tell client to send next file
			if _, err := rwc.Write([]byte{0, DlFldrActionNextFile}); err != nil {
				return err
			}
		} else {
			nextAction := DlFldrActionSendFile

			// Check if we have the full file already.  If so, send dlFldrAction_NextFile to client to skip.
			_, err := os.Stat(filepath.Join(fullPath, fu.FormattedPath()))
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			if err == nil {
				nextAction = DlFldrActionNextFile
			}

			//  Check if we have a partial file already.  If so, send dlFldrAction_ResumeFile to client to resume upload.
			incompleteFile, err := os.Stat(filepath.Join(fullPath, fu.FormattedPath()+IncompleteFileSuffix))
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			if err == nil {
				nextAction = DlFldrActionResumeFile
			}

			if _, err := rwc.Write([]byte{0, uint8(nextAction)}); err != nil {
				return err
			}

			switch nextAction {
			case DlFldrActionNextFile:
				continue
			case DlFldrActionResumeFile:
				offset := make([]byte, 4)
				binary.BigEndian.PutUint32(offset, uint32(incompleteFile.Size()))

				file, err := os.OpenFile(fullPath+"/"+fu.FormattedPath()+IncompleteFileSuffix, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					return err
				}

				fileResumeData := NewFileResumeData([]ForkInfoList{*NewForkInfoList(offset)})

				b, _ := fileResumeData.BinaryMarshal()

				bs := make([]byte, 2)
				binary.BigEndian.PutUint16(bs, uint16(len(b)))

				if _, err := rwc.Write(append(bs, b...)); err != nil {
					return err
				}

				if _, err := io.ReadFull(rwc, fileSize); err != nil {
					return err
				}

				if err := receiveFile(rwc, file, io.Discard, io.Discard, fileTransfer.bytesSentCounter); err != nil {
					rLogger.Error(err.Error())
				}

				err = os.Rename(fullPath+"/"+fu.FormattedPath()+".incomplete", fullPath+"/"+fu.FormattedPath())
				if err != nil {
					return err
				}

			case DlFldrActionSendFile:
				if _, err := io.ReadFull(rwc, fileSize); err != nil {
					return err
				}

				filePath := filepath.Join(fullPath, fu.FormattedPath())

				hlFile, err := NewFileWrapper(fileStore, filePath, 0)
				if err != nil {
					return err
				}

				rLogger.Info("Starting file transfer", "path", filePath, "fileNum", i+1, "fileSize", binary.BigEndian.Uint32(fileSize))

				incWriter, err := hlFile.incFileWriter()
				if err != nil {
					return err
				}

				rForkWriter := io.Discard
				iForkWriter := io.Discard
				if preserveForks {
					iForkWriter, err = hlFile.InfoForkWriter()
					if err != nil {
						return err
					}

					rForkWriter, err = hlFile.rsrcForkWriter()
					if err != nil {
						return err
					}
				}
				if err := receiveFile(rwc, incWriter, rForkWriter, iForkWriter, fileTransfer.bytesSentCounter); err != nil {
					return err
				}

				if err := os.Rename(filePath+".incomplete", filePath); err != nil {
					return err
				}
			}

			// Tell client to send next fileWrapper
			if _, err := rwc.Write([]byte{0, DlFldrActionNextFile}); err != nil {
				return err
			}
		}
	}
	rLogger.Info("Folder upload complete")
	return nil
}
