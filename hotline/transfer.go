package hotline

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
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

const fileCopyBufSize = 524288 // 512k
func receiveFile(conn io.Reader, targetFile io.Writer, resForkFile io.Writer) error {
	ffhBuf := make([]byte, 24)
	if _, err := conn.Read(ffhBuf); err != nil {
		return err
	}

	var ffh FlatFileHeader
	err := binary.Read(bytes.NewReader(ffhBuf), binary.BigEndian, &ffh)
	if err != nil {
		return err
	}

	ffifhBuf := make([]byte, 16)
	if _, err := conn.Read(ffifhBuf); err != nil {
		return err
	}

	var ffifh FlatFileInformationForkHeader
	err = binary.Read(bytes.NewReader(ffifhBuf), binary.BigEndian, &ffifh)
	if err != nil {
		return err
	}

	var ffif FlatFileInformationFork

	dataLen := binary.BigEndian.Uint32(ffifh.DataSize[:])
	ffifBuf := make([]byte, dataLen)
	if _, err := conn.Read(ffifBuf); err != nil {
		return err
	}
	if err := ffif.UnmarshalBinary(ffifBuf); err != nil {
		return err
	}

	var ffdfh FlatFileDataForkHeader
	ffdfhBuf := make([]byte, 16)
	if _, err := conn.Read(ffdfhBuf); err != nil {
		return err
	}
	err = binary.Read(bytes.NewReader(ffdfhBuf), binary.BigEndian, &ffdfh)
	if err != nil {
		return err
	}

	// this will be zero if the file only has a resource fork
	fileSize := int(binary.BigEndian.Uint32(ffdfh.DataSize[:]))

	bw := bufio.NewWriterSize(targetFile, fileCopyBufSize)
	_, err = io.CopyN(bw, conn, int64(fileSize))
	if err != nil {
		return err
	}
	if err := bw.Flush(); err != nil {
		return err
	}

	if ffh.ForkCount == [2]byte{0, 3} {
		var resForkHeader FlatFileDataForkHeader
		resForkBuf := make([]byte, 16)

		if _, err := conn.Read(resForkBuf); err != nil {
			return err
		}
		err = binary.Read(bytes.NewReader(resForkBuf), binary.BigEndian, &resForkHeader)
		if err != nil {
			return err
		}

		fileSize = int(binary.BigEndian.Uint32(resForkHeader.DataSize[:]))

		bw = bufio.NewWriterSize(resForkFile, fileCopyBufSize)
		_, err = io.CopyN(resForkFile, conn, int64(fileSize))
		if err != nil {
			return err
		}
		if err := bw.Flush(); err != nil {
			return err
		}
	}
	return nil
}
