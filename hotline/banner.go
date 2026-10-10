package hotline

import (
	"bytes"
	"math/rand/v2"
)

// BannerImage holds an image and its Hotline type code (JPEG or GIFf).
// Data returned by ClientConn.Banner is shared and must not be modified.
type BannerImage struct {
	Data     []byte
	TypeCode string
}

// SetBannerPool atomically replaces the images available to new directory-mode
// sessions. It copies the input; existing sessions and transfers keep their images.
// Banner and SetBanner continue to manage the separate single-file banner.
func (s *Server) SetBannerPool(images []BannerImage) {
	pool := make([]BannerImage, len(images))
	for i, img := range images {
		pool[i] = BannerImage{Data: bytes.Clone(img.Data), TypeCode: img.TypeCode}
	}
	s.bannerMu.Lock()
	defer s.bannerMu.Unlock()
	s.bannerPool = pool
}

// Banner returns the current single-file banner, or selects a directory-mode
// banner once for this session. A directory fallback is pinned in the same way.
// Callers must not modify the returned Data.
func (cc *ClientConn) Banner() BannerImage {
	return cc.selectBanner(rand.IntN)
}

// selectBanner takes an index chooser so selection can be tested deterministically.
func (cc *ClientConn) selectBanner(intN func(int) int) BannerImage {
	if cc.Server.Config.BannerDirectory == "" {
		return BannerImage{
			Data:     cc.Server.Banner(),
			TypeCode: FileTypeFromFilename(cc.Server.Config.BannerFile).TypeCode,
		}
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()
	if cc.banner == nil {
		cc.Server.bannerMu.RLock()
		defer cc.Server.bannerMu.RUnlock()
		if len(cc.Server.bannerPool) == 0 {
			return BannerImage{}
		}
		selected := cc.Server.bannerPool[intN(len(cc.Server.bannerPool))]
		cc.banner = &selected
	}
	return *cc.banner
}
