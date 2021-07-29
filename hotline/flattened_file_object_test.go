package hotline

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestReadFlattenedFileObject(t *testing.T) {
	testData, _ := hex.DecodeString("46494c500001000000000000000000000000000000000002494e464f000000000000000000000052414d414354455854747478740000000000000100000000000000000000000000000000000000000000000000000000000000000007700000ba74247307700000ba74247300000008746573742e74787400004441544100000000000000000000000474657374")

	ffo := ReadFlattenedFileObject(testData)

	format := ffo.FlatFileHeader.Format
	want := []byte("FILP")
	if !bytes.Equal(format, want) {
		t.Errorf("ReadFlattenedFileObject() = %q, want %q", format, want)
	}
}
//
//func TestNewFlattenedFileObject(t *testing.T) {
//	ffo := NewFlattenedFileObject("test/config/files", "testfile.txt")
//
//	dataSize := ffo.FlatFileDataForkHeader.DataSize
//	want := []byte{0, 0, 0, 0x17}
//	if bytes.Compare(dataSize, want) != 0 {
//		t.Errorf("%q, want %q", dataSize, want)
//	}
//
//	comment := ffo.FlatFileInformationFork.Comment
//	want = []byte("Test Comment")
//	if bytes.Compare(ffo.FlatFileInformationFork.Comment, want) != 0 {
//		t.Errorf("%q, want %q", comment, want)
//	}
//}
