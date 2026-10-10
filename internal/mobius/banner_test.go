package mobius

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestBanner(t *testing.T, dir, name, data string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(data), 0600))
}

func TestLoadBannerDirectory(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string]string{
		"a.JPG": "jpeg", "b.JpEg": "also jpeg", "c.GIF": "gif",
		".hidden.jpg": "hidden", "other.png": "png", "empty.gif": "",
	} {
		writeTestBanner(t, dir, name, data)
	}
	require.NoError(t, os.Mkdir(filepath.Join(dir, "nested.jpg"), 0700))
	writeTestBanner(t, filepath.Join(dir, "nested.jpg"), "inside.jpg", "nested")
	var logs bytes.Buffer
	r := NewBannerReloader(dir, &hotline.Server{Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	images, err := r.loadDirectory(dir)
	require.NoError(t, err)
	assert.Equal(t, []hotline.BannerImage{
		{Data: []byte("jpeg"), TypeCode: "JPEG"},
		{Data: []byte("also jpeg"), TypeCode: "JPEG"},
		{Data: []byte("gif"), TypeCode: "GIFf"},
	}, images)
	assert.Contains(t, logs.String(), "Skipping empty banner")

	unreadable := filepath.Join(dir, "unreadable.jpg")
	require.NoError(t, os.WriteFile(unreadable, []byte("unreadable"), 0000))
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0600) })
	if _, err := os.ReadFile(unreadable); err != nil {
		images, err = r.loadDirectory(dir)
		require.NoError(t, err)
		assert.Len(t, images, 3)
		assert.Contains(t, logs.String(), "Skipping unreadable banner")
	}

	t.Run("skips symlinks", func(t *testing.T) {
		dir := t.TempDir()
		writeTestBanner(t, dir, "banner.jpg", "jpeg")
		if err := os.Symlink(filepath.Join(dir, "banner.jpg"), filepath.Join(dir, "link.jpg")); err != nil {
			t.Skipf("symlink creation is unavailable: %v", err)
		}
		r := NewBannerReloader(dir, &hotline.Server{Logger: NewTestLogger()})
		images, err := r.loadDirectory(dir)
		require.NoError(t, err)
		assert.Equal(t, []hotline.BannerImage{{Data: []byte("jpeg"), TypeCode: "JPEG"}}, images)
	})
}

func TestBannerReloaderFallbackAndRecovery(t *testing.T) {
	for _, absolute := range []bool{false, true} {
		name := "relative"
		if absolute {
			name = "absolute"
		}
		t.Run(name, func(t *testing.T) {
			configDir := t.TempDir()
			dir := filepath.Join(configDir, "banners")
			configuredDir := "banners"
			if absolute {
				configuredDir = dir
			}
			var logs bytes.Buffer
			srv := &hotline.Server{
				Config: hotline.Config{BannerFile: "fallback.gif", BannerDirectory: configuredDir},
				Logger: slog.New(slog.NewTextHandler(&logs, nil)),
			}
			r := NewBannerReloader(configDir, srv)
			newSession := func() *hotline.ClientConn { return &hotline.ClientConn{Server: srv} }
			require.ErrorContains(t, r.Reload(), "BannerFile fallback")
			writeTestBanner(t, configDir, "fallback.gif", "fallback")
			require.NoError(t, r.Reload(), "missing directory uses fallback")
			fallbackSession := newSession()
			fallback := fallbackSession.Banner()
			assert.Equal(t, hotline.BannerImage{Data: []byte("fallback"), TypeCode: "GIFf"}, fallback)
			assert.Contains(t, logs.String(), "Unable to load banner directory")

			require.NoError(t, os.Mkdir(dir, 0700))
			require.NoError(t, r.Reload(), "empty directory uses fallback")
			assert.Equal(t, fallback, newSession().Banner())
			writeTestBanner(t, dir, "only.jpg", "directory banner")
			require.NoError(t, r.Reload())
			directorySession := newSession()
			selected := directorySession.Banner()
			assert.Equal(t, hotline.BannerImage{Data: []byte("directory banner"), TypeCode: "JPEG"}, selected)
			assert.Equal(t, fallback, fallbackSession.Banner())

			require.NoError(t, os.Remove(filepath.Join(dir, "only.jpg")))
			require.NoError(t, r.Reload(), "directory becoming empty switches new sessions to fallback")
			assert.Equal(t, fallback, newSession().Banner())
			assert.Equal(t, selected, directorySession.Banner())

			writeTestBanner(t, dir, "new.jpg", "new directory banner")
			require.NoError(t, r.Reload())
			current := newSession().Banner()
			require.NoError(t, os.Remove(filepath.Join(dir, "new.jpg")))
			require.NoError(t, os.Remove(filepath.Join(configDir, "fallback.gif")))
			require.Error(t, r.Reload())
			assert.Equal(t, current, newSession().Banner(), "both sources failing retains previous pool")
			assert.Equal(t, selected, directorySession.Banner())
		})
	}
}

func TestBannerReloaderDirectoryOnly(t *testing.T) {
	dir := t.TempDir()
	srv := &hotline.Server{Config: hotline.Config{BannerDirectory: "."}, Logger: NewTestLogger()}
	r := NewBannerReloader(dir, srv)
	require.ErrorContains(t, r.Reload(), "fallback is not configured")
	writeTestBanner(t, dir, "one.jpeg", "one")
	require.NoError(t, r.Reload())
	assert.Equal(t, "one", string((&hotline.ClientConn{Server: srv}).Banner().Data))

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 20 {
				assert.NoError(t, r.Reload())
				assert.Equal(t, "one", string((&hotline.ClientConn{Server: srv}).Banner().Data))
			}
		})
	}
	wg.Wait()
}

func TestBannerReloaderSingleFile(t *testing.T) {
	dir := t.TempDir()
	srv := &hotline.Server{Config: hotline.Config{BannerFile: "banner.jpg"}, Logger: NewTestLogger()}
	r := NewBannerReloader(dir, srv)
	require.Error(t, r.Reload())
	cc := &hotline.ClientConn{Server: srv}
	for _, data := range []string{"first", "replacement with different size", ""} {
		writeTestBanner(t, dir, "banner.jpg", data)
		require.NoError(t, r.Reload())
		assert.Equal(t, data, string(srv.Banner()))
		assert.Equal(t, data, string(cc.Banner().Data))
	}
	writeTestBanner(t, dir, "banner.jpg", "last working banner")
	require.NoError(t, r.Reload())
	require.NoError(t, os.Remove(filepath.Join(dir, "banner.jpg")))
	require.Error(t, r.Reload())
	assert.Equal(t, "last working banner", string(srv.Banner()))
}
