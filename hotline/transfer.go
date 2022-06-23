package hotline

import (
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

func receiveFile(r io.Reader, targetFile, resForkFile, infoFork, counterWriter io.Writer) error {
	var ffo flattenedFileObject
	if _, err := ffo.ReadFrom(r); err != nil {
		return err
	}

	// Write the information fork
	_, err := infoFork.Write(ffo.FlatFileInformationFork.MarshalBinary())
	if err != nil {
		return err
	}

	if _, err = io.Copy(targetFile, io.TeeReader(r, counterWriter)); err != nil {
		return err
	}

	if ffo.FlatFileHeader.ForkCount == [2]byte{0, 3} {
		if err := binary.Read(r, binary.BigEndian, &ffo.FlatFileResForkHeader); err != nil {
			return err
		}

		if _, err = io.Copy(resForkFile, io.TeeReader(r, counterWriter)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) bannerDownload(w io.Writer) error {
	bannerBytes, err := os.ReadFile(filepath.Join(s.ConfigDir, s.Config.BannerFile))
	if err != nil {
		return err
	}
	_, err = w.Write(bannerBytes)

	return err
}
