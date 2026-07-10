package hotline

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests exercise the file-transfer handlers (UploadHandler,
// DownloadFolderHandler, UploadFolderHandler) and, transitively, the fork
// writers in file_wrapper.go. They drive the real handlers against an
// OSFileStore backed by a temp dir so that the resource/info fork sidecar
// files land on disk and can be asserted.
//
// The folder handlers speak an interactive, lock-step protocol with the
// client. Those tests run the handler in a goroutine over one half of a
// net.Pipe and script the client on the other half.

// u32b encodes n as a big-endian [4]byte.
func u32b(n int) [4]byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(n))
	return b
}

// readWriter adapts a reader and writer into an io.ReadWriter for the transfer
// handlers. A nil Writer discards writes (UploadHandler never writes back).
type readWriter struct {
	r io.Reader
	w io.Writer
}

func (rw readWriter) Read(p []byte) (int, error) { return rw.r.Read(p) }
func (rw readWriter) Write(p []byte) (int, error) {
	if rw.w == nil {
		return len(p), nil
	}
	return rw.w.Write(p)
}

// flatFileBytes builds the on-wire flattened file object a client sends when
// uploading a file: FFO header + info fork + data fork header + data, followed
// by the resource fork header + resource bytes when rsrc is non-nil. This
// mirrors exactly what receiveFile consumes.
func flatFileBytes(name string, data, rsrc []byte) []byte {
	ffo := &flattenedFileObject{
		FlatFileHeader: FlatFileHeader{
			Format:    FormatFILP,
			Version:   [2]byte{0, 1},
			ForkCount: [2]byte{0, 2},
		},
		FlatFileInformationFork: NewFlatFileInformationFork(name, [8]byte{}, "TEXT", "TEXT"),
		FlatFileDataForkHeader: FlatFileForkHeader{
			ForkType: ForkTypeDATA,
			DataSize: u32b(len(data)),
		},
	}
	if rsrc != nil {
		ffo.FlatFileHeader.ForkCount = [2]byte{0, 3}
	}

	// flattenedFileObject.Read emits everything through the data fork header
	// but not the data itself or the resource fork.
	b, _ := io.ReadAll(ffo)
	b = append(b, data...)

	if rsrc != nil {
		var hdr bytes.Buffer
		_ = binary.Write(&hdr, binary.BigEndian, FlatFileForkHeader{ForkType: ForkTypeMACR, DataSize: u32b(len(rsrc))})
		b = append(b, hdr.Bytes()...)
		b = append(b, rsrc...)
	}
	return b
}

// writeInfoFork writes a valid .info_<name> sidecar so that NewFile reports a
// 3-fork file (ForkCount == 3), which is what gates the resource fork being
// sent during a folder download.
func writeInfoFork(t *testing.T, dir, name string) {
	t.Helper()
	info := NewFlatFileInformationFork(name, [8]byte{}, "TEXT", "TEXT")
	b, err := io.ReadAll(&info)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf(InfoForkNameTemplate, name)), b, 0644))
}

func rsrcPath(dir, name string) string {
	return filepath.Join(dir, fmt.Sprintf(RsrcForkNameTemplate, name))
}

func TestUploadHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("uploads a new file with data and resource forks", func(t *testing.T) {
		dir := t.TempDir()
		fileStore := &OSFileStore{}
		dst := filepath.Join(dir, "greeting.txt")

		data := []byte("the quick brown fox")
		rsrc := []byte("resource-fork-bytes")

		ft := &FileTransfer{bytesSentCounter: &WriteCounter{}}
		rwc := readWriter{r: bytes.NewReader(flatFileBytes("greeting.txt", data, rsrc))}

		require.NoError(t, UploadHandler(rwc, dst, ft, fileStore, logger, true))

		// Data fork committed to the final path; the .incomplete temp file is
		// renamed away.
		got, err := os.ReadFile(dst)
		require.NoError(t, err)
		assert.Equal(t, data, got)
		_, err = os.Stat(dst + IncompleteFileSuffix)
		assert.True(t, os.IsNotExist(err), "incomplete file should be renamed away")

		// Resource and info fork sidecars written.
		gotRsrc, err := os.ReadFile(rsrcPath(dir, "greeting.txt"))
		require.NoError(t, err)
		assert.Equal(t, rsrc, gotRsrc)
		_, err = os.Stat(filepath.Join(dir, fmt.Sprintf(InfoForkNameTemplate, "greeting.txt")))
		require.NoError(t, err, "info fork sidecar should be written")

		// The data bytes are counted through the transfer counter.
		assert.Equal(t, int64(len(data)+len(rsrc)), ft.bytesSentCounter.Total)
	})

	t.Run("uploads without preserving forks", func(t *testing.T) {
		dir := t.TempDir()
		fileStore := &OSFileStore{}
		dst := filepath.Join(dir, "plain.txt")
		data := []byte("plain data")

		ft := &FileTransfer{bytesSentCounter: &WriteCounter{}}
		rwc := readWriter{r: bytes.NewReader(flatFileBytes("plain.txt", data, nil))}

		require.NoError(t, UploadHandler(rwc, dst, ft, fileStore, logger, false))

		got, err := os.ReadFile(dst)
		require.NoError(t, err)
		assert.Equal(t, data, got)

		// preserveForks == false: no sidecar forks created.
		_, err = os.Stat(rsrcPath(dir, "plain.txt"))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("commits fork data on a backend that only persists on Close", func(t *testing.T) {
		// MemFileStore's writers buffer and only commit on Close. This guards
		// against regressing the fork writers being left unclosed, which would
		// silently drop the resource and info forks on such backends.
		fileStore := NewMemFileStore()
		dst := "/files/note.txt"

		data := []byte("body")
		rsrc := []byte("rsrc")

		ft := &FileTransfer{bytesSentCounter: &WriteCounter{}}
		rwc := readWriter{r: bytes.NewReader(flatFileBytes("note.txt", data, rsrc))}

		require.NoError(t, UploadHandler(rwc, dst, ft, fileStore, logger, true))

		got, err := fileStore.ReadFile(dst)
		require.NoError(t, err)
		assert.Equal(t, data, got)

		gotRsrc, err := fileStore.ReadFile("/files/" + fmt.Sprintf(RsrcForkNameTemplate, "note.txt"))
		require.NoError(t, err, "resource fork must be committed")
		assert.Equal(t, rsrc, gotRsrc)

		_, err = fileStore.ReadFile("/files/" + fmt.Sprintf(InfoForkNameTemplate, "note.txt"))
		require.NoError(t, err, "info fork must be committed")
	})

	t.Run("refuses to overwrite an existing file", func(t *testing.T) {
		dir := t.TempDir()
		fileStore := &OSFileStore{}
		dst := filepath.Join(dir, "exists.txt")
		require.NoError(t, os.WriteFile(dst, []byte("original"), 0644))

		ft := &FileTransfer{bytesSentCounter: &WriteCounter{}}
		rwc := readWriter{r: bytes.NewReader(flatFileBytes("exists.txt", []byte("new data"), nil))}

		err := UploadHandler(rwc, dst, ft, fileStore, logger, true)
		require.Error(t, err)

		// The existing file is left untouched.
		got, err := os.ReadFile(dst)
		require.NoError(t, err)
		assert.Equal(t, []byte("original"), got)
	})
}

// downloadedFile is a file a scripted folder-download client received.
type downloadedFile struct {
	name string
	data []byte
	rsrc []byte
	dir  bool
}

// readFolderDownloadHeader reads one FileHeader (Size[2] then Size bytes of
// Type[2] + FilePath) and returns the decoded relative name and whether it is a
// directory.
func readFolderDownloadHeader(t *testing.T, conn net.Conn) (name string, isDir bool) {
	t.Helper()
	var sizeBuf [2]byte
	_, err := io.ReadFull(conn, sizeBuf[:])
	require.NoError(t, err)
	body := make([]byte, binary.BigEndian.Uint16(sizeBuf[:]))
	_, err = io.ReadFull(conn, body)
	require.NoError(t, err)

	var fp FilePath
	_, err = fp.Write(body[2:])
	require.NoError(t, err)
	for _, item := range fp.Items {
		name = filepath.Join(name, string(item.Name))
	}
	return name, body[1] == 1
}

// runFolderDownloadClient scripts the client side of DownloadFolderHandler over
// conn, expecting exactly nExpected item headers, and returns what it received.
func runFolderDownloadClient(t *testing.T, conn net.Conn, nExpected int) []downloadedFile {
	t.Helper()

	// The server's first read is an unused pre-walk action.
	_, err := conn.Write([]byte{0, DlFldrActionSendFile})
	require.NoError(t, err)

	var got []downloadedFile
	for i := 0; i < nExpected; i++ {
		name, isDir := readFolderDownloadHeader(t, conn)

		if isDir {
			// Tell the server to advance to the next item.
			_, err = conn.Write([]byte{0, DlFldrActionNextFile})
			require.NoError(t, err)
			got = append(got, downloadedFile{name: name, dir: true})
			continue
		}

		// Request the file.
		_, err = conn.Write([]byte{0, DlFldrActionSendFile})
		require.NoError(t, err)

		// Server sends the 4-byte transfer size, then the flattened file
		// object, then the raw data fork, then (if 3 forks) the resource fork.
		var xferSize [4]byte
		_, err = io.ReadFull(conn, xferSize[:])
		require.NoError(t, err)

		var ffo flattenedFileObject
		_, err = ffo.ReadFrom(conn)
		require.NoError(t, err)

		data := make([]byte, ffo.dataSize())
		_, err = io.ReadFull(conn, data)
		require.NoError(t, err)

		var rsrc []byte
		if ffo.FlatFileHeader.ForkCount[1] == 3 {
			var rh FlatFileForkHeader
			require.NoError(t, binary.Read(conn, binary.BigEndian, &rh))
			rsrc = make([]byte, binary.BigEndian.Uint32(rh.DataSize[:]))
			_, err = io.ReadFull(conn, rsrc)
			require.NoError(t, err)
		}

		// Tell the server to advance to the next item.
		_, err = conn.Write([]byte{0, DlFldrActionNextFile})
		require.NoError(t, err)

		got = append(got, downloadedFile{name: name, data: data, rsrc: rsrc})
	}
	return got
}

func TestDownloadFolderHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("streams a folder tree including a subdir and a resource fork", func(t *testing.T) {
		dir := t.TempDir()
		fileStore := &OSFileStore{}

		root := filepath.Join(dir, "share")
		require.NoError(t, os.Mkdir(root, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("aaa"), 0644))
		// Give a.txt a resource fork (info sidecar flips ForkCount to 3).
		require.NoError(t, os.WriteFile(rsrcPath(root, "a.txt"), []byte("RRRR"), 0644))
		writeInfoFork(t, root, "a.txt")
		require.NoError(t, os.Mkdir(filepath.Join(root, "sub"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "sub", "b.txt"), []byte("bbbbb"), 0644))

		serverConn, clientConn := net.Pipe()
		require.NoError(t, clientConn.SetDeadline(time.Now().Add(10*time.Second)))

		ft := &FileTransfer{bytesSentCounter: &WriteCounter{}}
		errCh := make(chan error, 1)
		go func() {
			errCh <- DownloadFolderHandler(serverConn, root, ft, fileStore, logger, true)
			_ = serverConn.Close()
		}()

		// Walk visits: share, .rsrc_a.txt (dot, skipped), .info_a.txt (dot,
		// skipped), a.txt, sub, sub/b.txt -> 3 headers reach the client.
		got := runFolderDownloadClient(t, clientConn, 3)
		require.NoError(t, <-errCh)

		byName := map[string]downloadedFile{}
		for _, f := range got {
			byName[f.name] = f
		}

		require.Contains(t, byName, "a.txt")
		assert.Equal(t, []byte("aaa"), byName["a.txt"].data)
		assert.Equal(t, []byte("RRRR"), byName["a.txt"].rsrc)

		require.Contains(t, byName, "sub")
		assert.True(t, byName["sub"].dir)

		require.Contains(t, byName, filepath.Join("sub", "b.txt"))
		assert.Equal(t, []byte("bbbbb"), byName[filepath.Join("sub", "b.txt")].data)
	})

	t.Run("honors a resume request for a file", func(t *testing.T) {
		dir := t.TempDir()
		fileStore := &OSFileStore{}

		root := filepath.Join(dir, "share")
		require.NoError(t, os.Mkdir(root, 0755))
		fileData := []byte("0123456789")
		require.NoError(t, os.WriteFile(filepath.Join(root, "big.txt"), fileData, 0644))

		serverConn, clientConn := net.Pipe()
		require.NoError(t, clientConn.SetDeadline(time.Now().Add(10*time.Second)))

		ft := &FileTransfer{bytesSentCounter: &WriteCounter{}}
		errCh := make(chan error, 1)
		go func() {
			errCh <- DownloadFolderHandler(serverConn, root, ft, fileStore, logger, true)
			_ = serverConn.Close()
		}()

		// Pre-walk action.
		_, err := clientConn.Write([]byte{0, DlFldrActionSendFile})
		require.NoError(t, err)

		name, isDir := readFolderDownloadHeader(t, clientConn)
		require.Equal(t, "big.txt", name)
		require.False(t, isDir)

		// Ask to resume the file from offset 5.
		_, err = clientConn.Write([]byte{0, DlFldrActionResumeFile})
		require.NoError(t, err)

		offset := u32b(5)
		frd := NewFileResumeData([]ForkInfoList{*NewForkInfoList(offset[:])})
		resumeBytes, err := frd.BinaryMarshal()
		require.NoError(t, err)
		lenBuf := make([]byte, 2)
		binary.BigEndian.PutUint16(lenBuf, uint16(len(resumeBytes)))
		_, err = clientConn.Write(append(lenBuf, resumeBytes...))
		require.NoError(t, err)

		// Server replies with the transfer size, the flattened file object, and
		// the data fork.
		var xferSize [4]byte
		_, err = io.ReadFull(clientConn, xferSize[:])
		require.NoError(t, err)

		var ffo flattenedFileObject
		_, err = ffo.ReadFrom(clientConn)
		require.NoError(t, err)
		data := make([]byte, ffo.dataSize())
		_, err = io.ReadFull(clientConn, data)
		require.NoError(t, err)

		// Advance past the (single) file so the walk completes.
		_, err = clientConn.Write([]byte{0, DlFldrActionNextFile})
		require.NoError(t, err)

		require.NoError(t, <-errCh)
		assert.Equal(t, fileData, data)
	})
}

// folderUploadItem describes one item a scripted client offers during an
// UploadFolderHandler run.
type folderUploadItem struct {
	relPath string // e.g. "top.txt" or "sub/inner.txt"
	isDir   bool
	data    []byte
	rsrc    []byte
}

// encodeFolderUploadPath encodes relPath into the FileNamePath wire form:
// for each segment, a 2-byte separator + 1-byte length + segment bytes.
func encodeFolderUploadPath(relPath string) (pathItemCount int, encoded []byte) {
	var buf bytes.Buffer
	var count int
	for _, seg := range strings.Split(filepath.ToSlash(relPath), "/") {
		if seg == "" {
			continue
		}
		buf.Write([]byte{0, 0})       // separator
		buf.WriteByte(byte(len(seg))) // segment length
		buf.WriteString(seg)          // segment bytes
		count++
	}
	return count, buf.Bytes()
}

// runFolderUploadClient scripts the client side of UploadFolderHandler.
func runFolderUploadClient(t *testing.T, conn net.Conn, items []folderUploadItem) {
	t.Helper()

	// Server opens the flow by telling the client to send the first item.
	action := make([]byte, 2)
	_, err := io.ReadFull(conn, action)
	require.NoError(t, err)

	for _, item := range items {
		pathCount, encPath := encodeFolderUploadPath(item.relPath)

		dataSize := len(encPath) + 4
		hdr := make([]byte, 0, dataSize+2)
		hdr = append(hdr, byte(dataSize>>8), byte(dataSize)) // DataSize[2]
		if item.isDir {
			hdr = append(hdr, 0, 1) // IsFolder
		} else {
			hdr = append(hdr, 0, 0)
		}
		hdr = append(hdr, byte(pathCount>>8), byte(pathCount)) // PathItemCount[2]
		hdr = append(hdr, encPath...)
		_, err := conn.Write(hdr)
		require.NoError(t, err)

		// Read the server's next-action response.
		_, err = io.ReadFull(conn, action)
		require.NoError(t, err)

		if item.isDir {
			continue // server Mkdir'd and told us to advance
		}

		switch action[1] {
		case DlFldrActionNextFile:
			// Server already has this file; nothing more to send.
			continue
		case DlFldrActionSendFile:
			base := filepath.Base(item.relPath)
			size := u32b(len(item.data))
			_, err = conn.Write(size[:])
			require.NoError(t, err)
			_, err = conn.Write(flatFileBytes(base, item.data, item.rsrc))
			require.NoError(t, err)

			// Server tells us to advance to the next item.
			_, err = io.ReadFull(conn, action)
			require.NoError(t, err)
		default:
			t.Fatalf("unexpected server action %v", action)
		}
	}
}

func TestUploadFolderHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("receives a folder tree with a nested file and resource fork", func(t *testing.T) {
		dir := t.TempDir()
		fileStore := &OSFileStore{}
		dst := filepath.Join(dir, "incoming")

		items := []folderUploadItem{
			{relPath: "top.txt", data: []byte("TOP"), rsrc: []byte("TR")},
			{relPath: "sub", isDir: true},
			{relPath: filepath.Join("sub", "inner.txt"), data: []byte("INNER")},
		}

		ft := &FileTransfer{
			bytesSentCounter: &WriteCounter{},
			FolderItemCount:  []byte{0, byte(len(items))},
		}

		serverConn, clientConn := net.Pipe()
		require.NoError(t, clientConn.SetDeadline(time.Now().Add(10*time.Second)))

		errCh := make(chan error, 1)
		go func() {
			errCh <- UploadFolderHandler(serverConn, dst, ft, fileStore, logger, true)
			_ = serverConn.Close()
		}()

		runFolderUploadClient(t, clientConn, items)
		require.NoError(t, <-errCh)

		top, err := os.ReadFile(filepath.Join(dst, "top.txt"))
		require.NoError(t, err)
		assert.Equal(t, []byte("TOP"), top)

		gotRsrc, err := os.ReadFile(rsrcPath(dst, "top.txt"))
		require.NoError(t, err)
		assert.Equal(t, []byte("TR"), gotRsrc)

		subInfo, err := os.Stat(filepath.Join(dst, "sub"))
		require.NoError(t, err)
		assert.True(t, subInfo.IsDir())

		inner, err := os.ReadFile(filepath.Join(dst, "sub", "inner.txt"))
		require.NoError(t, err)
		assert.Equal(t, []byte("INNER"), inner)
	})

	t.Run("skips a file that already exists", func(t *testing.T) {
		dir := t.TempDir()
		fileStore := &OSFileStore{}
		dst := filepath.Join(dir, "incoming")
		require.NoError(t, os.Mkdir(dst, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dst, "dup.txt"), []byte("keep me"), 0644))

		items := []folderUploadItem{
			{relPath: "dup.txt", data: []byte("should be ignored")},
		}

		ft := &FileTransfer{
			bytesSentCounter: &WriteCounter{},
			FolderItemCount:  []byte{0, byte(len(items))},
		}

		serverConn, clientConn := net.Pipe()
		require.NoError(t, clientConn.SetDeadline(time.Now().Add(10*time.Second)))

		errCh := make(chan error, 1)
		go func() {
			errCh <- UploadFolderHandler(serverConn, dst, ft, fileStore, logger, true)
			_ = serverConn.Close()
		}()

		runFolderUploadClient(t, clientConn, items)
		require.NoError(t, <-errCh)

		// The pre-existing file is left untouched.
		got, err := os.ReadFile(filepath.Join(dst, "dup.txt"))
		require.NoError(t, err)
		assert.Equal(t, []byte("keep me"), got)
	})

	t.Run("resumes a partially uploaded file", func(t *testing.T) {
		dir := t.TempDir()
		fileStore := &OSFileStore{}
		dst := filepath.Join(dir, "incoming")
		require.NoError(t, os.Mkdir(dst, 0755))
		// A prior, interrupted upload left a partial .incomplete file.
		require.NoError(t, os.WriteFile(filepath.Join(dst, "resume.txt"+IncompleteFileSuffix), []byte("012"), 0644))

		ft := &FileTransfer{
			bytesSentCounter: &WriteCounter{},
			FolderItemCount:  []byte{0, 1},
		}

		serverConn, clientConn := net.Pipe()
		require.NoError(t, clientConn.SetDeadline(time.Now().Add(10*time.Second)))

		errCh := make(chan error, 1)
		go func() {
			errCh <- UploadFolderHandler(serverConn, dst, ft, fileStore, logger, true)
			_ = serverConn.Close()
		}()

		action := make([]byte, 2)
		_, err := io.ReadFull(clientConn, action) // initial "send next" from server
		require.NoError(t, err)

		// Offer the file the server already has partially.
		pathCount, encPath := encodeFolderUploadPath("resume.txt")
		dataSize := len(encPath) + 4
		hdr := []byte{byte(dataSize >> 8), byte(dataSize), 0, 0, byte(pathCount >> 8), byte(pathCount)}
		hdr = append(hdr, encPath...)
		_, err = clientConn.Write(hdr)
		require.NoError(t, err)

		// Server detects the partial file and asks to resume.
		_, err = io.ReadFull(clientConn, action)
		require.NoError(t, err)
		require.Equal(t, byte(DlFldrActionResumeFile), action[1])

		// Server sends resume data (2-byte length prefix + payload); consume it.
		resumeLen := make([]byte, 2)
		_, err = io.ReadFull(clientConn, resumeLen)
		require.NoError(t, err)
		resumeData := make([]byte, binary.BigEndian.Uint16(resumeLen))
		_, err = io.ReadFull(clientConn, resumeData)
		require.NoError(t, err)

		// Send the remaining data fork; it is appended to the partial file.
		size := u32b(3)
		_, err = clientConn.Write(size[:])
		require.NoError(t, err)
		_, err = clientConn.Write(flatFileBytes("resume.txt", []byte("345"), nil))
		require.NoError(t, err)

		// Server tells us to advance, completing the run.
		_, err = io.ReadFull(clientConn, action)
		require.NoError(t, err)

		require.NoError(t, <-errCh)

		got, err := os.ReadFile(filepath.Join(dst, "resume.txt"))
		require.NoError(t, err)
		assert.Equal(t, []byte("012345"), got)
	})
}

func TestFileTransfer_ItemCount(t *testing.T) {
	ft := &FileTransfer{FolderItemCount: []byte{0x01, 0x00}}
	assert.Equal(t, 256, ft.ItemCount())
}

func TestMemFileTransferMgr_Lifecycle(t *testing.T) {
	cc := &ClientConn{ClientFileTransferMgr: NewClientFileTransferMgr()}
	cc.Server = &Server{FileTransferMgr: NewMemFileTransferMgr()}

	ft := cc.NewFileTransfer(FileDownload, "/root", []byte("f"), []byte("p"), []byte{0, 0, 0, 1})
	require.NotNil(t, ft)

	// Add assigns a random reference number and registers the transfer.
	got := cc.Server.FileTransferMgr.Get(ft.RefNum)
	require.NotNil(t, got)
	assert.Equal(t, ft, got)
	assert.Len(t, cc.ClientFileTransferMgr.Get(FileDownload), 1)

	cc.Server.FileTransferMgr.Delete(ft.RefNum)
	assert.Nil(t, cc.Server.FileTransferMgr.Get(ft.RefNum))
	assert.Empty(t, cc.ClientFileTransferMgr.Get(FileDownload))
}
