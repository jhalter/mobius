package hotline

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// R2FileStore is a FileStore backed by Cloudflare R2 via its S3-compatible API. It follows the
// same model MemFileStore uses: a flat keyspace of cleaned paths where directories are derived
// from key prefixes, plus zero-byte marker objects (key + "/") so that empty Hotline folders
// persist. Symlinks/aliases have no object-store analog and return errors.ErrUnsupported.
//
// R2 has no append operation, but the file library opens partially-uploaded ".incomplete" files
// with O_APPEND and derives the resume offset from their size. R2FileStore therefore routes every
// ".incomplete" path to a local staging store (real filesystem, real append) and only promotes the
// finished object to R2 on the terminal Rename(x.incomplete -> x) that commits an upload. All other
// paths — data forks and their .rsrc_/.info_ sidecars, directories — live in R2.
type R2FileStore struct {
	api      s3API
	uploader s3Uploader
	bucket   string
	prefix   string // optional key prefix within the bucket (no surrounding slashes)
	staging  *stagingStore
}

var _ FileStore = (*R2FileStore)(nil)

// s3API is the subset of *s3.Client that R2FileStore calls. Narrowing it to an interface lets tests
// inject a fake without a network or real bucket.
type s3API interface {
	HeadObject(ctx context.Context, in *s3.HeadObjectInput, opts ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	GetObject(ctx context.Context, in *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(ctx context.Context, in *s3.PutObjectInput, opts ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObject(ctx context.Context, in *s3.DeleteObjectInput, opts ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	DeleteObjects(ctx context.Context, in *s3.DeleteObjectsInput, opts ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
	CopyObject(ctx context.Context, in *s3.CopyObjectInput, opts ...func(*s3.Options)) (*s3.CopyObjectOutput, error)
	ListObjectsV2(ctx context.Context, in *s3.ListObjectsV2Input, opts ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

// s3Uploader streams a body to R2 as a (multipart, if large) upload. *manager.Uploader satisfies it.
type s3Uploader interface {
	Upload(ctx context.Context, in *s3.PutObjectInput, opts ...func(*manager.Uploader)) (*manager.UploadOutput, error)
}

// NewR2FileStore builds an R2-backed FileStore from a configured S3 client. prefix is an optional
// key prefix within the bucket; stagingDir is a local directory used to buffer in-progress
// (.incomplete) uploads before they are promoted to R2.
func NewR2FileStore(client *s3.Client, bucket, prefix, stagingDir string) *R2FileStore {
	return newR2FileStore(client, manager.NewUploader(client), bucket, prefix, stagingDir)
}

func newR2FileStore(api s3API, up s3Uploader, bucket, prefix, stagingDir string) *R2FileStore {
	return &R2FileStore{
		api:      api,
		uploader: up,
		bucket:   bucket,
		prefix:   strings.Trim(filepath.ToSlash(prefix), "/"),
		staging:  &stagingStore{root: stagingDir},
	}
}

// key maps a file-library path to an R2 object key: forward-slashed, leading separator stripped,
// with the optional configured prefix prepended.
func (s *R2FileStore) key(name string) string {
	p := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(name)), "/")
	if s.prefix != "" {
		if p == "" {
			return s.prefix
		}
		return s.prefix + "/" + p
	}
	return p
}

func isIncomplete(name string) bool {
	return strings.HasSuffix(name, IncompleteFileSuffix)
}

// dirPrefix is the listing prefix for the children of a directory key. The root key ("") lists the
// whole bucket (or configured prefix).
func dirPrefix(key string) string {
	if key == "" {
		return ""
	}
	return key + "/"
}

// --- Reads -----------------------------------------------------------------------------------

func (s *R2FileStore) Open(name string) (io.ReadCloser, error) {
	if isIncomplete(name) {
		return s.staging.Open(name)
	}
	out, err := s.api.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(name)),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	return out.Body, nil
}

func (s *R2FileStore) Stat(name string) (fs.FileInfo, error) {
	if isIncomplete(name) {
		return s.staging.Stat(name)
	}
	key := s.key(name)
	if fi, err := s.headInfo(key); err == nil {
		return fi, nil
	} else if !isNotFound(err) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: err}
	}
	ok, err := s.dirExists(key)
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: err}
	}
	if ok {
		return r2FileInfo{name: path.Base(key), isDir: true}, nil
	}
	return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
}

func (s *R2FileStore) ReadFile(name string) ([]byte, error) {
	if isIncomplete(name) {
		r, err := s.staging.Open(name)
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(r)
	}
	r, err := s.Open(name)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func (s *R2FileStore) ReadDir(name string) ([]fs.DirEntry, error) {
	key := s.key(name)

	// A file is not a directory (mirrors MemFileStore).
	if _, err := s.headInfo(key); err == nil {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: errors.New("not a directory")}
	} else if !isNotFound(err) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: err}
	}

	prefix := dirPrefix(key)
	var entries []fs.DirEntry
	var token *string
	for {
		out, err := s.api.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(prefix),
			Delimiter:         aws.String("/"),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, &fs.PathError{Op: "readdir", Path: name, Err: err}
		}
		for _, cp := range out.CommonPrefixes {
			base := path.Base(strings.TrimSuffix(aws.ToString(cp.Prefix), "/"))
			entries = append(entries, fs.FileInfoToDirEntry(r2FileInfo{name: base, isDir: true}))
		}
		for _, obj := range out.Contents {
			// Skip the directory's own marker object.
			if aws.ToString(obj.Key) == prefix {
				continue
			}
			entries = append(entries, fs.FileInfoToDirEntry(r2FileInfo{
				name:    path.Base(aws.ToString(obj.Key)),
				size:    aws.ToInt64(obj.Size),
				modTime: aws.ToTime(obj.LastModified),
			}))
		}
		if aws.ToBool(out.IsTruncated) {
			token = out.NextContinuationToken
			continue
		}
		break
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

func (s *R2FileStore) ReadLink(name string) (string, error) {
	return "", &fs.PathError{Op: "readlink", Path: name, Err: errors.ErrUnsupported}
}

// Walk mirrors filepath.Walk / MemFileStore.Walk: it visits root and all descendants in lexical
// order, synthesizing directory entries for key prefixes that have no explicit marker object, and
// honors filepath.SkipDir returned from fn.
func (s *R2FileStore) Walk(root string, fn filepath.WalkFunc) error {
	cleanRoot := filepath.Clean(root)
	rootKey := s.key(root)

	// If the root is itself an object, Walk visits just that file.
	if fi, err := s.headInfo(rootKey); err == nil {
		return fn(cleanRoot, fi, nil)
	} else if !isNotFound(err) {
		return err
	}

	objs, err := s.listAll(dirPrefix(rootKey))
	if err != nil {
		return err
	}
	if len(objs) == 0 {
		return fn(cleanRoot, nil, &fs.PathError{Op: "lstat", Path: cleanRoot, Err: fs.ErrNotExist})
	}

	// Build the visitable set: the root plus every object, synthesizing intermediate directories.
	infos := map[string]fs.FileInfo{cleanRoot: r2FileInfo{name: path.Base(cleanRoot), isDir: true}}
	for _, o := range objs {
		rel := strings.Trim(strings.TrimPrefix(o.key, rootKey), "/")
		if rel == "" {
			continue // the root's own marker
		}
		isDir := strings.HasSuffix(o.key, "/")
		segs := strings.Split(rel, "/")
		for i := 0; i < len(segs)-1; i++ {
			dp := cleanRoot + "/" + strings.Join(segs[:i+1], "/")
			if _, ok := infos[dp]; !ok {
				infos[dp] = r2FileInfo{name: segs[i], isDir: true}
			}
		}
		leaf := cleanRoot + "/" + rel
		if isDir {
			if _, ok := infos[leaf]; !ok {
				infos[leaf] = r2FileInfo{name: segs[len(segs)-1], isDir: true}
			}
		} else {
			infos[leaf] = r2FileInfo{name: segs[len(segs)-1], size: o.size, modTime: o.modTime}
		}
	}

	paths := make([]string, 0, len(infos))
	for p := range infos {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var skipped []string
	for _, p := range paths {
		skip := false
		for _, sk := range skipped {
			if p == sk || strings.HasPrefix(p, sk+"/") {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		info := infos[p]
		if err := fn(p, info, nil); err != nil {
			if errors.Is(err, filepath.SkipDir) && info.IsDir() {
				skipped = append(skipped, p)
				continue
			}
			return err
		}
	}
	return nil
}

// --- Writes ----------------------------------------------------------------------------------

func (s *R2FileStore) Create(name string) (io.WriteCloser, error) {
	return s.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
}

func (s *R2FileStore) OpenFile(name string, flag int, perm fs.FileMode) (io.WriteCloser, error) {
	if isIncomplete(name) {
		return s.staging.OpenFile(name, flag, perm)
	}
	// Non-incomplete writes (data forks written via Create, .rsrc_/.info_ sidecars) stream straight
	// to R2. There is no append use of R2 keys — only the staged .incomplete path uses O_APPEND.
	return s.newUploadWriter(s.key(name)), nil
}

func (s *R2FileStore) WriteFile(name string, data []byte, perm fs.FileMode) error {
	if isIncomplete(name) {
		w, err := s.staging.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
		if err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			_ = w.Close()
			return err
		}
		return w.Close()
	}
	_, err := s.api.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(name)),
		Body:   bytes.NewReader(data),
	})
	return err
}

func (s *R2FileStore) Mkdir(name string, perm fs.FileMode) error {
	key := s.key(name)
	if _, err := s.headInfo(key); err == nil {
		return &fs.PathError{Op: "mkdir", Path: name, Err: fs.ErrExist}
	} else if !isNotFound(err) {
		return &fs.PathError{Op: "mkdir", Path: name, Err: err}
	}
	_, err := s.api.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key + "/"),
		Body:   bytes.NewReader(nil),
	})
	return err
}

// --- Mutations -------------------------------------------------------------------------------

func (s *R2FileStore) Rename(oldpath, newpath string) error {
	oldInc, newInc := isIncomplete(oldpath), isIncomplete(newpath)
	switch {
	case oldInc && newInc:
		// Move of a still-incomplete upload within the staging area (File.Move).
		return s.staging.Rename(oldpath, newpath)
	case oldInc && !newInc:
		// The upload commit: promote the staged .incomplete file to a finished R2 object.
		return s.promote(oldpath, newpath)
	case !oldInc && !newInc:
		return s.renameR2(oldpath, newpath)
	default:
		// R2 -> staging is never produced by the file library.
		return &fs.PathError{Op: "rename", Path: oldpath, Err: errors.ErrUnsupported}
	}
}

func (s *R2FileStore) Remove(name string) error {
	if isIncomplete(name) {
		return s.staging.Remove(name)
	}
	// DeleteObject is idempotent on R2 (deleting a missing key succeeds), which matches how callers
	// tolerate os.ErrNotExist when removing optional sidecar forks.
	_, err := s.api.DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(name)),
	})
	return err
}

func (s *R2FileStore) RemoveAll(name string) error {
	if isIncomplete(name) {
		return s.staging.RemoveAll(name)
	}
	key := s.key(name)
	children, err := s.listAll(dirPrefix(key))
	if err != nil {
		return err
	}
	keys := []string{key}
	for _, o := range children {
		keys = append(keys, o.key)
	}
	return s.deleteKeys(keys)
}

func (s *R2FileStore) Symlink(oldname, newname string) error {
	return &fs.PathError{Op: "symlink", Path: newname, Err: errors.ErrUnsupported}
}

// --- Rename helpers --------------------------------------------------------------------------

// promote streams a finished, locally-staged .incomplete file up to its final R2 key, then removes
// the local temp. If the upload fails the staged file is left in place so the transfer can resume.
func (s *R2FileStore) promote(oldName, newName string) error {
	full := s.staging.full(oldName)
	f, err := os.Open(full)
	if err != nil {
		return err // already fs.ErrNotExist-compatible
	}
	defer f.Close()

	if _, err := s.uploader.Upload(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(newName)),
		Body:   f,
	}); err != nil {
		return fmt.Errorf("promote %q to R2: %w", oldName, err)
	}
	_ = f.Close()
	return os.Remove(full)
}

// renameR2 moves an object, or a whole directory subtree, within R2 (copy + delete, since object
// stores have no rename). Single-object renames back the data/rsrc/info fork moves in File.Move;
// the directory branch backs folder renames.
func (s *R2FileStore) renameR2(oldName, newName string) error {
	oldKey, newKey := s.key(oldName), s.key(newName)

	if _, err := s.headInfo(oldKey); err == nil {
		if err := s.copyObject(oldKey, newKey); err != nil {
			return err
		}
		_, err := s.api.DeleteObject(context.Background(), &s3.DeleteObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(oldKey),
		})
		return err
	} else if !isNotFound(err) {
		return &fs.PathError{Op: "rename", Path: oldName, Err: err}
	}

	objs, err := s.listAll(dirPrefix(oldKey))
	if err != nil {
		return err
	}
	if len(objs) == 0 {
		return &fs.PathError{Op: "rename", Path: oldName, Err: fs.ErrNotExist}
	}
	oldKeys := make([]string, 0, len(objs))
	for _, o := range objs {
		if err := s.copyObject(o.key, newKey+strings.TrimPrefix(o.key, oldKey)); err != nil {
			return err
		}
		oldKeys = append(oldKeys, o.key)
	}
	return s.deleteKeys(oldKeys)
}

func (s *R2FileStore) copyObject(srcKey, dstKey string) error {
	_, err := s.api.CopyObject(context.Background(), &s3.CopyObjectInput{
		Bucket:     aws.String(s.bucket),
		Key:        aws.String(dstKey),
		CopySource: aws.String(copySource(s.bucket, srcKey)),
	})
	return err
}

// copySource builds the URL-encoded "bucket/key" value S3/R2 expects in x-amz-copy-source,
// preserving path separators while escaping spaces and other special characters in the key.
func copySource(bucket, key string) string {
	return (&url.URL{Path: bucket + "/" + key}).EscapedPath()
}

// --- S3 helpers ------------------------------------------------------------------------------

func (s *R2FileStore) headInfo(key string) (fs.FileInfo, error) {
	out, err := s.api.HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return r2FileInfo{
		name:    path.Base(key),
		size:    aws.ToInt64(out.ContentLength),
		modTime: aws.ToTime(out.LastModified),
	}, nil
}

// dirExists reports whether any object lives under the directory key — either the explicit marker
// created by Mkdir or any descendant object of a non-empty directory.
func (s *R2FileStore) dirExists(key string) (bool, error) {
	out, err := s.api.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
		Bucket:  aws.String(s.bucket),
		Prefix:  aws.String(dirPrefix(key)),
		MaxKeys: aws.Int32(1),
	})
	if err != nil {
		return false, err
	}
	return len(out.Contents) > 0, nil
}

type r2Object struct {
	key     string
	size    int64
	modTime time.Time
}

func (s *R2FileStore) listAll(prefix string) ([]r2Object, error) {
	var out []r2Object
	var token *string
	for {
		resp, err := s.api.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, err
		}
		for _, o := range resp.Contents {
			out = append(out, r2Object{
				key:     aws.ToString(o.Key),
				size:    aws.ToInt64(o.Size),
				modTime: aws.ToTime(o.LastModified),
			})
		}
		if aws.ToBool(resp.IsTruncated) {
			token = resp.NextContinuationToken
			continue
		}
		return out, nil
	}
}

func (s *R2FileStore) deleteKeys(keys []string) error {
	const batch = 1000 // S3 DeleteObjects limit
	for i := 0; i < len(keys); i += batch {
		end := i + batch
		if end > len(keys) {
			end = len(keys)
		}
		objs := make([]types.ObjectIdentifier, 0, end-i)
		for _, k := range keys[i:end] {
			objs = append(objs, types.ObjectIdentifier{Key: aws.String(k)})
		}
		if _, err := s.api.DeleteObjects(context.Background(), &s3.DeleteObjectsInput{
			Bucket: aws.String(s.bucket),
			Delete: &types.Delete{Objects: objs, Quiet: aws.Bool(true)},
		}); err != nil {
			return err
		}
	}
	return nil
}

// newUploadWriter returns an io.WriteCloser that streams to R2 via a multipart upload running in a
// background goroutine. Memory use is bounded to the uploader's part size × concurrency rather than
// the whole object. Close flushes and returns the upload's result.
func (s *R2FileStore) newUploadWriter(key string) io.WriteCloser {
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() {
		_, err := s.uploader.Upload(context.Background(), &s3.PutObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(key),
			Body:   pr,
		})
		// Unblock any pending Write if the upload aborted early.
		_ = pr.CloseWithError(err)
		done <- err
	}()
	return &s3PipeWriter{pw: pw, done: done}
}

type s3PipeWriter struct {
	pw   *io.PipeWriter
	done chan error
}

func (w *s3PipeWriter) Write(p []byte) (int, error) {
	return w.pw.Write(p)
}

func (w *s3PipeWriter) Close() error {
	// Signal EOF to the uploader, then wait for it to finish and surface its error.
	if err := w.pw.Close(); err != nil {
		return err
	}
	return <-w.done
}

// isNotFound reports whether an S3 error means the object (or bucket key) does not exist, across the
// several shapes the SDK returns it in (typed NoSuchKey/NotFound, a smithy APIError code, or a bare
// HTTP 404 from HeadObject).
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var nf *types.NotFound
	if errors.As(err, &nf) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return true
		}
	}
	var respErr *awshttp.ResponseError
	if errors.As(err, &respErr) && respErr.HTTPStatusCode() == 404 {
		return true
	}
	return false
}

// r2FileInfo implements fs.FileInfo over R2 object metadata (or a synthesized directory).
type r2FileInfo struct {
	name    string
	size    int64
	modTime time.Time
	isDir   bool
}

func (fi r2FileInfo) Name() string { return fi.name }
func (fi r2FileInfo) Size() int64  { return fi.size }
func (fi r2FileInfo) Mode() fs.FileMode {
	if fi.isDir {
		return fs.ModeDir | 0755
	}
	return 0644
}
func (fi r2FileInfo) ModTime() time.Time { return fi.modTime }
func (fi r2FileInfo) IsDir() bool        { return fi.isDir }
func (fi r2FileInfo) Sys() any           { return nil }

// stagingStore is a small local-filesystem store used to buffer in-progress .incomplete uploads,
// rooted at a temp directory. It gives real O_APPEND and cheap size-based resume that R2 cannot,
// and creates parent directories on write (the transfer path never explicitly Mkdirs first).
type stagingStore struct {
	root string
}

// full maps a library path to a path inside the staging root, anchored so it cannot escape.
func (s *stagingStore) full(name string) string {
	return filepath.Join(s.root, filepath.FromSlash(filepath.Clean("/"+filepath.ToSlash(name))))
}

func (s *stagingStore) OpenFile(name string, flag int, perm fs.FileMode) (io.WriteCloser, error) {
	full := s.full(name)
	if err := os.MkdirAll(filepath.Dir(full), 0750); err != nil {
		return nil, err
	}
	return os.OpenFile(full, flag, perm)
}

func (s *stagingStore) Stat(name string) (fs.FileInfo, error) { return os.Stat(s.full(name)) }
func (s *stagingStore) Open(name string) (io.ReadCloser, error) {
	return os.Open(s.full(name))
}
func (s *stagingStore) Remove(name string) error    { return os.Remove(s.full(name)) }
func (s *stagingStore) RemoveAll(name string) error { return os.RemoveAll(s.full(name)) }

func (s *stagingStore) Rename(oldpath, newpath string) error {
	newFull := s.full(newpath)
	if err := os.MkdirAll(filepath.Dir(newFull), 0750); err != nil {
		return err
	}
	return os.Rename(s.full(oldpath), newFull)
}
