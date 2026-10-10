package hotline

import (
	"bytes"
	"context"
	"encoding/binary"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBannerSelection(t *testing.T) {
	images := []BannerImage{
		{Data: []byte("jpeg"), TypeCode: "JPEG"},
		{Data: []byte("longer gif"), TypeCode: "GIFf"},
	}
	srv := &Server{Config: Config{BannerDirectory: "banners"}}
	srv.SetBannerPool(images)
	for index, want := range images {
		cc := &ClientConn{Server: srv}
		calls := 0
		choose := func(n int) int {
			assert.Equal(t, len(images), n)
			calls++
			return index
		}
		assert.Equal(t, want, cc.selectBanner(choose))
		assert.Equal(t, want, cc.selectBanner(choose))
		assert.Equal(t, 1, calls, "choose only once per session")
		srv.SetBannerPool([]BannerImage{{Data: []byte("replacement"), TypeCode: "JPEG"}})
		assert.Equal(t, want, cc.Banner(), "existing session survives pool replacement")
		assert.Equal(t, "replacement", string((&ClientConn{Server: srv}).Banner().Data))
		srv.SetBannerPool(images)
	}

	// Publishing copies input data so callers cannot later change a session's image.
	images[0].Data[0] = 'X'
	assert.Equal(t, "jpeg", string((&ClientConn{Server: srv}).selectBanner(func(int) int { return 0 }).Data))

	empty := &ClientConn{Server: &Server{Config: Config{BannerDirectory: "banners"}}}
	assert.Empty(t, empty.Banner())
	empty.Server.SetBannerPool([]BannerImage{{Data: []byte("now available"), TypeCode: "JPEG"}})
	assert.Equal(t, "now available", string(empty.Banner().Data))
}

func TestBannerSelectionConcurrentReload(t *testing.T) {
	srv := &Server{Config: Config{BannerDirectory: "banners"}}
	images := []BannerImage{{Data: []byte("jpeg"), TypeCode: "JPEG"}, {Data: []byte("gif"), TypeCode: "GIFf"}}
	srv.SetBannerPool(images[:1])
	shared := &ClientConn{Server: srv}
	want := shared.Banner()
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := range 200 {
			srv.SetBannerPool(images[i%2 : i%2+1])
		}
	})
	for range 4 {
		wg.Go(func() {
			for range 200 {
				cc := &ClientConn{Server: srv}
				got := cc.Banner()
				assert.Contains(t, images, got)
				assert.Equal(t, got, cc.Banner())
				assert.Equal(t, want, shared.Banner())
			}
		})
	}
	wg.Wait()
}

// Inspect each banner before publication to catch initialization after Add.
type inspectingBannerTransferMgr struct {
	FileTransferMgr
	inspect func(*FileTransfer)
}

func (m inspectingBannerTransferMgr) Add(ft *FileTransfer) {
	m.inspect(ft)
	m.FileTransferMgr.Add(ft)
}

func TestBannerTransferSnapshot(t *testing.T) {
	for _, directory := range []string{"", "banners"} {
		t.Run("directory="+directory, func(t *testing.T) {
			srv, err := NewServer(WithConfig(Config{BannerFile: "banner.jpg", BannerDirectory: directory}), WithLogger(NewTestLogger()))
			require.NoError(t, err)
			original := []byte("original banner")
			replacement := []byte("replacement with a different size")
			srv.SetBanner(original)
			srv.SetBannerPool([]BannerImage{{Data: original, TypeCode: "GIFf"}})
			srv.FileTransferMgr = inspectingBannerTransferMgr{
				FileTransferMgr: NewMemFileTransferMgr(),
				inspect: func(ft *FileTransfer) {
					assert.Equal(t, original, ft.bannerData)
					assert.Equal(t, uint32(len(original)), binary.BigEndian.Uint32(ft.TransferSize))
				},
			}
			cc := &ClientConn{Server: srv, Account: &Account{Login: "guest"}, ClientFileTransferMgr: NewClientFileTransferMgr()}
			ft := cc.NewFileTransfer(BannerDownload, "", nil, nil, nil)
			srv.SetBanner(replacement)
			srv.SetBannerPool([]BannerImage{{Data: replacement, TypeCode: "JPEG"}})

			var header, payload bytes.Buffer
			require.NoError(t, binary.Write(&header, binary.BigEndian, transfer{Protocol: HTXF, ReferenceNumber: ft.RefNum}))
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Skip the legacy transfer-close delay.
			require.NoError(t, srv.handleFileTransfer(ctx, readWriter{r: &header, w: &payload}))
			assert.Equal(t, original, payload.Bytes())
			assert.Nil(t, srv.FileTransferMgr.Get(ft.RefNum))
			if directory == "" {
				assert.Equal(t, replacement, cc.Banner().Data, "single-file sessions see successful reloads")
			} else {
				assert.Equal(t, original, cc.Banner().Data, "directory sessions keep their selection")
			}
		})
	}
}
