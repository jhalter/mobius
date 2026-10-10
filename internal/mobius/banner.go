package mobius

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jhalter/mobius/hotline"
)

// BannerReloader loads local banner images independently of the file library's
// storage backend. A failed reload leaves the previous images in service.
type BannerReloader struct {
	server          *hotline.Server
	configDir       string
	bannerFile      string
	bannerDirectory string
	mu              sync.Mutex
}

func NewBannerReloader(configDir string, server *hotline.Server) *BannerReloader {
	return &BannerReloader{
		server: server, configDir: configDir,
		bannerFile: server.Config.BannerFile, bannerDirectory: server.Config.BannerDirectory,
	}
}

// Reload serializes loading and publication so overlapping reloads cannot
// publish an older read after a newer one.
func (r *BannerReloader) Reload() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.bannerDirectory == "" {
		data, err := os.ReadFile(path.Join(r.configDir, r.bannerFile))
		if err != nil {
			return err
		}
		r.server.SetBanner(data)
		return nil
	}

	dir := r.bannerDirectory
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(r.configDir, dir)
	}
	images, err := r.loadDirectory(dir)
	if err != nil {
		r.server.Logger.Warn("Unable to load banner directory", "directory", dir, "err", err, "fallback", r.bannerFile)
		if r.bannerFile == "" {
			return fmt.Errorf("load banner directory: %w; BannerFile fallback is not configured", err)
		}
		// Keep the existing BannerFile path semantics, including for fallback.
		data, fallbackErr := os.ReadFile(path.Join(r.configDir, r.bannerFile))
		if fallbackErr != nil {
			return fmt.Errorf("load banner directory: %w; BannerFile fallback: %w", err, fallbackErr)
		}
		images = []hotline.BannerImage{{Data: data, TypeCode: hotline.FileTypeFromFilename(r.bannerFile).TypeCode}}
	}
	r.server.SetBannerPool(images)
	return nil
}

func (r *BannerReloader) loadDirectory(dir string) ([]hotline.BannerImage, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var images []hotline.BannerImage
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") || !entry.Type().IsRegular() || !isBannerFilename(entry.Name()) {
			continue
		}
		filename := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(filename)
		if err != nil {
			r.server.Logger.Warn("Skipping unreadable banner", "file", filename, "err", err)
			continue
		}
		if len(data) == 0 {
			r.server.Logger.Warn("Skipping empty banner", "file", filename)
			continue
		}
		images = append(images, hotline.BannerImage{Data: data, TypeCode: hotline.FileTypeFromFilename(entry.Name()).TypeCode})
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("no usable .jpg, .jpeg, or .gif images in %s", dir)
	}
	return images, nil
}

func isBannerFilename(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".gif"
}
