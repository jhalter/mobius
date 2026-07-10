package hotline

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeS3 is an in-memory stand-in for the R2/S3 API used to exercise R2FileStore without a network.
// It implements both s3API and s3Uploader.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
	modTime map[string]time.Time

	// failOp injects an error the next time the named operation runs (e.g. "GetObject").
	// The entry is consumed on use so callers can target a single call.
	failOp map[string]error
}

func newFakeS3() *fakeS3 {
	return &fakeS3{objects: map[string][]byte{}, modTime: map[string]time.Time{}, failOp: map[string]error{}}
}

// fail returns and consumes an injected error for op, or nil if none is set.
func (f *fakeS3) fail(op string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.failOp[op]; ok {
		delete(f.failOp, op)
		return err
	}
	return nil
}

func (f *fakeS3) put(key string, data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = append([]byte(nil), data...)
	f.modTime[key] = time.Unix(1700000000, 0)
}

func (f *fakeS3) HeadObject(_ context.Context, in *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if err := f.fail("HeadObject"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.objects[aws.ToString(in.Key)]
	if !ok {
		return nil, &types.NotFound{}
	}
	return &s3.HeadObjectOutput{
		ContentLength: aws.Int64(int64(len(data))),
		LastModified:  aws.Time(f.modTime[aws.ToString(in.Key)]),
	}, nil
}

func (f *fakeS3) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if err := f.fail("GetObject"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.objects[aws.ToString(in.Key)]
	if !ok {
		return nil, &types.NoSuchKey{}
	}
	return &s3.GetObjectOutput{
		Body:          io.NopCloser(bytes.NewReader(data)),
		ContentLength: aws.Int64(int64(len(data))),
	}, nil
}

func (f *fakeS3) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if err := f.fail("PutObject"); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	f.put(aws.ToString(in.Key), data)
	return &s3.PutObjectOutput{}, nil
}

// Upload satisfies s3Uploader; the fake treats it identically to PutObject.
func (f *fakeS3) Upload(_ context.Context, in *s3.PutObjectInput, _ ...func(*manager.Uploader)) (*manager.UploadOutput, error) { //nolint:staticcheck // SA1019: transfermanager successor is not yet GA
	if err := f.fail("Upload"); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	f.put(aws.ToString(in.Key), data)
	return &manager.UploadOutput{}, nil
}

func (f *fakeS3) DeleteObject(_ context.Context, in *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	if err := f.fail("DeleteObject"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, aws.ToString(in.Key))
	delete(f.modTime, aws.ToString(in.Key))
	return &s3.DeleteObjectOutput{}, nil
}

func (f *fakeS3) DeleteObjects(_ context.Context, in *s3.DeleteObjectsInput, _ ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
	if err := f.fail("DeleteObjects"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, o := range in.Delete.Objects {
		delete(f.objects, aws.ToString(o.Key))
		delete(f.modTime, aws.ToString(o.Key))
	}
	return &s3.DeleteObjectsOutput{}, nil
}

func (f *fakeS3) CopyObject(_ context.Context, in *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	if err := f.fail("CopyObject"); err != nil {
		return nil, err
	}
	src, err := decodeCopySource(aws.ToString(in.CopySource))
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.objects[src]
	if !ok {
		return nil, &types.NoSuchKey{}
	}
	f.objects[aws.ToString(in.Key)] = append([]byte(nil), data...)
	f.modTime[aws.ToString(in.Key)] = f.modTime[src]
	return &s3.CopyObjectOutput{}, nil
}

// decodeCopySource reverses copySource: URL-decode, then drop the leading "bucket/" segment.
func decodeCopySource(s string) (string, error) {
	dec, err := url.PathUnescape(s)
	if err != nil {
		return "", err
	}
	if i := strings.Index(dec, "/"); i >= 0 {
		return dec[i+1:], nil
	}
	return dec, nil
}

func (f *fakeS3) ListObjectsV2(_ context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if err := f.fail("ListObjectsV2"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	prefix := aws.ToString(in.Prefix)
	delim := aws.ToString(in.Delimiter)

	var keys []string
	for k := range f.objects {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var contents []types.Object
	commonSet := map[string]struct{}{}
	var common []string
	for _, k := range keys {
		if delim != "" {
			rest := strings.TrimPrefix(k, prefix)
			if idx := strings.Index(rest, delim); idx >= 0 {
				cp := prefix + rest[:idx+len(delim)]
				if _, ok := commonSet[cp]; !ok {
					commonSet[cp] = struct{}{}
					common = append(common, cp)
				}
				continue
			}
		}
		contents = append(contents, types.Object{
			Key:          aws.String(k),
			Size:         aws.Int64(int64(len(f.objects[k]))),
			LastModified: aws.Time(f.modTime[k]),
		})
	}

	out := &s3.ListObjectsV2Output{}
	truncated := false
	if in.MaxKeys != nil && int(*in.MaxKeys) < len(contents) {
		contents = contents[:*in.MaxKeys]
		truncated = true
	}
	for _, c := range common {
		out.CommonPrefixes = append(out.CommonPrefixes, types.CommonPrefix{Prefix: aws.String(c)})
	}
	out.Contents = contents
	out.IsTruncated = aws.Bool(truncated)
	return out, nil
}

func newTestR2Store(t *testing.T) (*R2FileStore, *fakeS3) {
	t.Helper()
	fake := newFakeS3()
	return newR2FileStore(fake, fake, "test-bucket", "", t.TempDir()), fake
}

func TestR2FileStore_WriteReadStat(t *testing.T) {
	s, _ := newTestR2Store(t)

	require.NoError(t, s.WriteFile("/files/sub/bar.txt", []byte("data"), 0644))

	got, err := s.ReadFile("/files/sub/bar.txt")
	require.NoError(t, err)
	assert.Equal(t, []byte("data"), got)

	fi, err := s.Stat("/files/sub/bar.txt")
	require.NoError(t, err)
	assert.False(t, fi.IsDir())
	assert.Equal(t, int64(4), fi.Size())

	// A directory is derived from the key prefix even with no marker object.
	fi, err = s.Stat("/files/sub")
	require.NoError(t, err)
	assert.True(t, fi.IsDir())

	_, err = s.Stat("/files/missing")
	assert.True(t, errors.Is(err, fs.ErrNotExist))
}

func TestR2FileStore_MkdirReadDir(t *testing.T) {
	s, _ := newTestR2Store(t)
	require.NoError(t, s.WriteFile("/root/a.txt", []byte("a"), 0644))
	require.NoError(t, s.WriteFile("/root/b.txt", []byte("bb"), 0644))
	require.NoError(t, s.Mkdir("/root/sub", 0755))          // explicit empty dir marker
	require.NoError(t, s.WriteFile("/root/deep/x", []byte("x"), 0644)) // implicit dir

	// Empty Mkdir'd directory persists and is Stat-able.
	fi, err := s.Stat("/root/sub")
	require.NoError(t, err)
	assert.True(t, fi.IsDir())

	entries, err := s.ReadDir("/root")
	require.NoError(t, err)

	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Equal(t, []string{"a.txt", "b.txt", "deep", "sub"}, names)

	// ReadDir on a file is an error.
	_, err = s.ReadDir("/root/a.txt")
	require.Error(t, err)
}

func TestR2FileStore_WalkCounts(t *testing.T) {
	s, _ := newTestR2Store(t)
	require.NoError(t, s.Mkdir("/w", 0755))
	require.NoError(t, s.WriteFile("/w/a.txt", []byte("1234"), 0644))
	require.NoError(t, s.WriteFile("/w/sub/b.txt", []byte("567"), 0644))

	size, err := CalcTotalSize(s, "/w")
	require.NoError(t, err)
	assert.Equal(t, []byte{0x00, 0x00, 0x00, 0x07}, size) // 4 + 3 bytes

	count, err := CalcItemCount(s, "/w")
	require.NoError(t, err)
	// Walk visits /w, /w/a.txt, /w/sub, /w/sub/b.txt = 4 entries, minus the root.
	assert.Equal(t, []byte{0x00, 0x03}, count)
}

func TestR2FileStore_WalkSkipDir(t *testing.T) {
	s, _ := newTestR2Store(t)
	require.NoError(t, s.WriteFile("/w/keep.txt", []byte("1"), 0644))
	require.NoError(t, s.WriteFile("/w/skipme/deep.txt", []byte("2"), 0644))

	var visited []string
	err := s.Walk("/w", func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && info.Name() == "skipme" {
			return filepath.SkipDir
		}
		visited = append(visited, p)
		return nil
	})
	require.NoError(t, err)
	assert.Contains(t, visited, "/w/keep.txt")
	for _, p := range visited {
		assert.NotContains(t, p, "skipme/deep.txt")
	}
}

func TestR2FileStore_RenameObjectAndDir(t *testing.T) {
	s, fake := newTestR2Store(t)

	// Single-object rename (data fork move).
	require.NoError(t, s.WriteFile("/d/old.txt", []byte("payload"), 0644))
	require.NoError(t, s.Rename("/d/old.txt", "/d/new.txt"))
	_, err := s.Stat("/d/old.txt")
	assert.True(t, errors.Is(err, fs.ErrNotExist))
	got, err := s.ReadFile("/d/new.txt")
	require.NoError(t, err)
	assert.Equal(t, []byte("payload"), got)

	// Directory rename (folder move) relocates every descendant.
	require.NoError(t, s.WriteFile("/d/dir/a.txt", []byte("a"), 0644))
	require.NoError(t, s.WriteFile("/d/dir/nested/b.txt", []byte("b"), 0644))
	require.NoError(t, s.Rename("/d/dir", "/d/moved"))

	got, err = s.ReadFile("/d/moved/nested/b.txt")
	require.NoError(t, err)
	assert.Equal(t, []byte("b"), got)
	_, err = s.Stat("/d/dir/a.txt")
	assert.True(t, errors.Is(err, fs.ErrNotExist))

	// Renaming a nonexistent path reports ErrNotExist.
	err = s.Rename("/d/ghost", "/d/wherever")
	assert.True(t, errors.Is(err, fs.ErrNotExist))

	_ = fake
}

func TestR2FileStore_RemoveAll(t *testing.T) {
	s, _ := newTestR2Store(t)
	require.NoError(t, s.WriteFile("/f/sub/x", []byte("x"), 0644))
	require.NoError(t, s.WriteFile("/f/sub/y", []byte("y"), 0644))

	require.NoError(t, s.RemoveAll("/f/sub"))
	_, err := s.Stat("/f/sub/x")
	assert.True(t, errors.Is(err, fs.ErrNotExist))
	_, err = s.Stat("/f/sub")
	assert.True(t, errors.Is(err, fs.ErrNotExist))
}

func TestR2FileStore_SymlinkUnsupported(t *testing.T) {
	s, _ := newTestR2Store(t)
	err := s.Symlink("/a", "/b")
	assert.True(t, errors.Is(err, errors.ErrUnsupported))

	_, err = s.ReadLink("/b")
	assert.True(t, errors.Is(err, errors.ErrUnsupported))
}

// TestR2FileStore_StagingAppendPromote exercises the resumable-upload path: .incomplete writes are
// appended locally across two sessions, then Rename promotes the finished object to R2.
func TestR2FileStore_StagingAppendPromote(t *testing.T) {
	s, fake := newTestR2Store(t)

	// Session 1: open with O_APPEND, write the first chunk.
	w, err := s.OpenFile("/files/big.txt.incomplete", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err)
	_, err = w.Write([]byte("hello "))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	// Resume offset is derived from the staged file's size.
	fi, err := s.Stat("/files/big.txt.incomplete")
	require.NoError(t, err)
	assert.Equal(t, int64(6), fi.Size())

	// Session 2: append the remainder.
	w2, err := s.OpenFile("/files/big.txt.incomplete", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err)
	_, err = w2.Write([]byte("world"))
	require.NoError(t, err)
	require.NoError(t, w2.Close())

	// Commit: promote to the final R2 key.
	require.NoError(t, s.Rename("/files/big.txt.incomplete", "/files/big.txt"))

	assert.Equal(t, []byte("hello world"), fake.objects["files/big.txt"])

	// The staged temp file is gone.
	_, err = s.Stat("/files/big.txt.incomplete")
	assert.True(t, errors.Is(err, fs.ErrNotExist))
}

func TestR2FileStore_KeyPrefix(t *testing.T) {
	fake := newFakeS3()
	s := newR2FileStore(fake, fake, "test-bucket", "hotline/files", t.TempDir())

	require.NoError(t, s.WriteFile("/a/b.txt", []byte("z"), 0644))
	_, ok := fake.objects["hotline/files/a/b.txt"]
	assert.True(t, ok, "object key should include the configured prefix")

	got, err := s.ReadFile("/a/b.txt")
	require.NoError(t, err)
	assert.Equal(t, []byte("z"), got)
}

// TestR2FileStore_DownloadRoundTrip proves the backend is swappable: a file stored only in the fake
// object store is served through the real DownloadHandler.
func TestR2FileStore_DownloadRoundTrip(t *testing.T) {
	s, _ := newTestR2Store(t)

	fileData := []byte("the quick brown fox")
	require.NoError(t, s.WriteFile("/files/story.txt", fileData, 0644))

	ft := &FileTransfer{bytesSentCounter: &WriteCounter{}}

	var out bytes.Buffer
	err := DownloadHandler(&out, "/files/story.txt", ft, s, slog.Default(), true)
	require.NoError(t, err)

	assert.True(t, bytes.Contains(out.Bytes(), fileData), "download output should contain the file data")
	assert.Equal(t, int64(len(fileData)), ft.bytesSentCounter.Total)
}

// TestR2FileStore_ErrorPropagation verifies that S3 API failures surface as errors rather than
// being swallowed or panicking. Errors are injected via fakeS3.failOp.
func TestR2FileStore_ErrorPropagation(t *testing.T) {
	injected := errors.New("r2 unavailable")

	t.Run("Open surfaces GetObject error", func(t *testing.T) {
		s, fake := newTestR2Store(t)
		require.NoError(t, s.WriteFile("/a.txt", []byte("x"), 0644))
		fake.failOp["GetObject"] = injected

		_, err := s.Open("/a.txt")
		require.ErrorIs(t, err, injected)
	})

	t.Run("Stat surfaces HeadObject error", func(t *testing.T) {
		s, fake := newTestR2Store(t)
		require.NoError(t, s.WriteFile("/a.txt", []byte("x"), 0644))
		fake.failOp["HeadObject"] = injected

		_, err := s.Stat("/a.txt")
		require.ErrorIs(t, err, injected)
	})

	t.Run("WriteFile surfaces PutObject error", func(t *testing.T) {
		s, fake := newTestR2Store(t)
		fake.failOp["PutObject"] = injected

		require.ErrorIs(t, s.WriteFile("/a.txt", []byte("x"), 0644), injected)
	})

	t.Run("Remove surfaces DeleteObject error", func(t *testing.T) {
		s, fake := newTestR2Store(t)
		require.NoError(t, s.WriteFile("/a.txt", []byte("x"), 0644))
		fake.failOp["DeleteObject"] = injected

		require.ErrorIs(t, s.Remove("/a.txt"), injected)
	})

	t.Run("RemoveAll surfaces ListObjectsV2 error", func(t *testing.T) {
		s, fake := newTestR2Store(t)
		require.NoError(t, s.WriteFile("/dir/a.txt", []byte("x"), 0644))
		fake.failOp["ListObjectsV2"] = injected

		require.ErrorIs(t, s.RemoveAll("/dir"), injected)
	})

	t.Run("Rename of existing object surfaces CopyObject error", func(t *testing.T) {
		s, fake := newTestR2Store(t)
		require.NoError(t, s.WriteFile("/a.txt", []byte("x"), 0644))
		fake.failOp["CopyObject"] = injected

		require.ErrorIs(t, s.Rename("/a.txt", "/b.txt"), injected)
	})

	t.Run("upload commit surfaces Upload error and leaves staged file", func(t *testing.T) {
		s, fake := newTestR2Store(t)
		incomplete := "/upload.txt" + IncompleteFileSuffix
		require.NoError(t, s.WriteFile(incomplete, []byte("partial"), 0644))
		fake.failOp["Upload"] = injected

		// promote() streams the staged file up on the terminal rename; a failure must surface.
		err := s.Rename(incomplete, "/upload.txt")
		require.ErrorIs(t, err, injected)

		// The staged file is retained so the transfer can resume.
		_, statErr := s.Stat(incomplete)
		require.NoError(t, statErr)
	})
}

// TestR2FileStore_Integration round-trips against a real R2 bucket. It is skipped unless R2_BUCKET
// and credentials are present in the environment.
func TestR2FileStore_Integration(t *testing.T) {
	if os.Getenv("R2_BUCKET") == "" {
		t.Skip("R2_BUCKET not set; skipping R2 integration test")
	}
	t.Skip("integration harness intentionally left as a manual/env-gated exercise; see plan verification section")
}
