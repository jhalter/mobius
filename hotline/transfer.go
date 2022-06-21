package hotline

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
)

type transfer struct {
	Protocol        [4]byte // "HTXF" 0x48545846
	ReferenceNumber [4]byte // Unique ID generated for the transfer
	DataSize        [4]byte // File size
	RSVD            [4]byte // Not implemented in Hotline Protocol
}

var HTXF = [4]byte{0x48, 0x54, 0x58, 0x46} // (HTXF) is the only supported transfer protocol

func (tf *transfer) Write(b []byte) (int, error) {
	if err := binary.Read(bytes.NewReader(b), binary.BigEndian, tf); err != nil {
		return 0, err
	}

	if tf.Protocol != HTXF {
		return 0, errors.New("invalid protocol")
	}

	return len(b), nil
}

const fileCopyBufSize = 4096

func receiveFile(r io.Reader, targetFile, resForkFile, infoFork io.Writer) error {
	var ffo flattenedFileObject
	if _, err := ffo.ReadFrom(r); err != nil {
		return err
	}

	// Write the information fork
	_, err := infoFork.Write(ffo.FlatFileInformationFork.MarshalBinary())
	if err != nil {
		return err
	}

	// read and write the data fork
	bw := bufio.NewWriterSize(targetFile, fileCopyBufSize)
	if _, err = io.CopyN(bw, r, ffo.dataSize()); err != nil {
		return err
	}
	if err := bw.Flush(); err != nil {
		return err
	}

	if ffo.FlatFileHeader.ForkCount == [2]byte{0, 3} {
		if err := binary.Read(r, binary.BigEndian, &ffo.FlatFileResForkHeader); err != nil {
			return err
		}

		bw = bufio.NewWriterSize(resForkFile, fileCopyBufSize)
		_, err = io.CopyN(resForkFile, r, ffo.rsrcSize())
		if err != nil {
			return err
		}
		if err := bw.Flush(); err != nil {
			return err
		}
	}
	return nil
}

func sendFile(w io.Writer, r io.Reader, offset int) (err error) {
	br := bufio.NewReader(r)
	if _, err := br.Discard(offset); err != nil {
		return err
	}

	rSendBuffer := make([]byte, 1024)
	for {
		var bytesRead int

		if bytesRead, err = br.Read(rSendBuffer); err == io.EOF {
			if _, err := w.Write(rSendBuffer[:bytesRead]); err != nil {
				return err
			}
			return nil
		}
		if err != nil {
			return err
		}
		// totalSent += int64(bytesRead)

		// fileTransfer.BytesSent += bytesRead

		if _, err := w.Write(rSendBuffer[:bytesRead]); err != nil {
			return err
		}
	}
}

func (s *Server) bannerDownload(w io.Writer) error {
	bannerBytes, err := os.ReadFile(filepath.Join(s.ConfigDir, s.Config.BannerFile))
	if err != nil {
		return err
	}
	_, err = w.Write(bannerBytes)

	return err
}
