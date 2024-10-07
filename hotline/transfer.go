package hotline

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"io"
)

type transfer struct {
	Protocol        [4]byte // "HTXF" 0x48545846
	ReferenceNumber [4]byte // Unique Type generated for the transfer
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

func receiveFile(r io.Reader, file *File, counterWriter io.Writer) error {
	var ffo FlattenedFileObject
	if _, err := ffo.ReadFrom(r); err != nil {
		return fmt.Errorf("read flattened file object: %v", err)
	}

	spew.Dump(ffo, file.InfoFork)

	// Write the information fork
	_, err := io.Copy(file.InfoFork, &ffo.FlatFileInformationFork)
	if err != nil {
		return fmt.Errorf("write the information fork: %v", err)
	}
	//err = file.InfoFork.Close()
	//if err != nil {
	//	spew.Dump(err)
	//}

	if _, err = io.CopyN(file.DataFork, io.TeeReader(r, counterWriter), ffo.dataSize()); err != nil {
		return fmt.Errorf("copy file data to partial file: %v", err)
	}
	spew.Dump(file.InfoFork)
	spew.Dump(ffo.FlatFileHeader.ForkCount)

	if ffo.FlatFileHeader.ForkCount == [2]byte{0, 3} {
		if err := binary.Read(r, binary.BigEndian, &ffo.FlatFileResForkHeader); err != nil {
			return fmt.Errorf("read resource fork header: %v", err)
		}

		spew.Dump("ffo.rsrcSize()", ffo.rsrcSize())
		if _, err = io.CopyN(file.RsrcFork, io.TeeReader(r, counterWriter), ffo.rsrcSize()); err != nil {
			return fmt.Errorf("read resource fork: %v", err)
		}
	}
	return nil
}
