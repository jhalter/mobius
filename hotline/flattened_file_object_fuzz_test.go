package hotline

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

// sampleFFOBytes encodes a small flattened file object (FILP header, INFO fork header,
// information fork, DATA fork header) — the layout that ReadFrom parses off the wire.
func sampleFFOBytes(t interface{ Fatal(...any) }) []byte {
	ffo := flattenedFileObject{
		FlatFileHeader: FlatFileHeader{
			Format:    [4]byte{'F', 'I', 'L', 'P'},
			Version:   [2]byte{0, 1},
			ForkCount: [2]byte{0, 2},
		},
		FlatFileInformationFork: NewFlatFileInformationFork("testfile.txt", [8]byte{}, "TEXT", "TTXT"),
		FlatFileDataForkHeader: FlatFileForkHeader{
			ForkType: [4]byte{'D', 'A', 'T', 'A'},
			DataSize: [4]byte{0, 0, 0, 5},
		},
	}
	b, err := io.ReadAll(&ffo)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// FuzzFlatFileInformationForkUnmarshal feeds raw untrusted bytes to the information fork
// decoder, which parses the metadata section of client file uploads.
func FuzzFlatFileInformationForkUnmarshal(f *testing.F) {
	fork := NewFlatFileInformationFork("testfile.txt", [8]byte{}, "TEXT", "TTXT")
	forkBytes, err := io.ReadAll(&fork)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(forkBytes)
	f.Add(forkBytes[:flatFileInfoForkMinLen]) // fixed-size section only
	f.Add([]byte{})                           // empty

	f.Fuzz(func(t *testing.T, data []byte) {
		var ffif FlatFileInformationFork
		if err := ffif.UnmarshalBinary(data); err != nil {
			return
		}
		// A successfully decoded fork must re-encode without panicking.
		_, err := io.ReadAll(&ffif)
		require.NoError(t, err)
	})
}

// FuzzFlattenedFileObjectReadFrom feeds raw untrusted bytes to the flattened file object
// parser — the entry point for decoding client file uploads on the transfer port.
func FuzzFlattenedFileObjectReadFrom(f *testing.F) {
	sample := sampleFFOBytes(f)
	f.Add(sample)
	f.Add(sample[:24]) // FILP header only
	// Information fork header declaring an absurd size (previously triggered an unbounded allocation).
	huge := bytes.Clone(sample)
	copy(huge[36:40], []byte{0xff, 0xff, 0xff, 0xff})
	f.Add(huge)

	f.Fuzz(func(t *testing.T, data []byte) {
		var ffo flattenedFileObject
		_, _ = ffo.ReadFrom(bytes.NewReader(data)) // must not panic or over-allocate
	})
}

// TestFlattenedFileObject_ReadFrom_RoundTrip pins that ReadFrom can parse what Read encodes.
func TestFlattenedFileObject_ReadFrom_RoundTrip(t *testing.T) {
	sample := sampleFFOBytes(t)

	var ffo flattenedFileObject
	_, err := ffo.ReadFrom(bytes.NewReader(sample))
	require.NoError(t, err)

	require.Equal(t, [4]byte{'F', 'I', 'L', 'P'}, ffo.FlatFileHeader.Format)
	require.Equal(t, []byte("testfile.txt"), ffo.FlatFileInformationFork.Name)
	require.Equal(t, ForkType{'D', 'A', 'T', 'A'}, ffo.FlatFileDataForkHeader.ForkType)
	require.Equal(t, int64(5), ffo.dataSize())
}

// TestFlattenedFileObject_ReadFrom_RejectsOversizedInfoFork pins the untrusted-size guard.
func TestFlattenedFileObject_ReadFrom_RejectsOversizedInfoFork(t *testing.T) {
	sample := sampleFFOBytes(t)
	copy(sample[36:40], []byte{0xff, 0xff, 0xff, 0xff})

	var ffo flattenedFileObject
	_, err := ffo.ReadFrom(bytes.NewReader(sample))
	require.ErrorContains(t, err, "exceeds maximum")
}
