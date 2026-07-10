package mobius

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/jhalter/mobius/hotline"
	"github.com/jhalter/mobius/hotline/hltest"
	"github.com/stretchr/testify/mock"
	"golang.org/x/text/encoding/charmap"
)

func TestHandleGetMsgs(t *testing.T) {
	type args struct {
		cc *hotline.ClientConn
		t  hotline.Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []hotline.Transaction
	}{
		{
			name: "returns news data",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessNewsReadArt)
							return bits
						}(),
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						MessageBoard: func() *mockReadWriteSeeker {
							m := mockReadWriteSeeker{}
							m.On("Seek", int64(0), 0).Return(int64(0), nil)
							m.On("Read", mock.AnythingOfType("[]uint8")).Run(func(args mock.Arguments) {
								arg := args.Get(0).([]uint8)
								copy(arg, "TEST")
							}).Return(4, io.EOF)
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranGetMsgs, [2]byte{0, 1},
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply: 0x01,
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldData, []byte("TEST")),
					},
				},
			},
		},
		{
			name: "when ReadAll returns an error",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessNewsReadArt)
							return bits
						}(),
					},
					Logger: NewTestLogger(),
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						MessageBoard: func() *mockReadWriteSeeker {
							m := mockReadWriteSeeker{}
							m.On("Seek", int64(0), 0).Return(int64(0), nil)
							m.On("Read", mock.AnythingOfType("[]uint8")).Return(0, errors.New("read error"))
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranGetMsgs, [2]byte{0, 1},
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("Error reading message board.")),
					},
				},
			},
		},
		{
			name: "when user does not have required permission",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: hotline.AccessBitmap{},
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						//Accounts: map[string]*Account{},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranGetMsgs, [2]byte{0, 1},
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to read news.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleGetMsgs(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleDelNewsArt(t *testing.T) {
	type args struct {
		cc *hotline.ClientConn
		t  hotline.Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []hotline.Transaction
	}{
		{
			name: "without required permission",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							return bits
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranDelNewsArt,
					[2]byte{0, 0},
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to delete news articles.")),
					},
				},
			},
		},
		{
			name: "when DeleteArticle returns an error",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessNewsDeleteArt)
							return bits
						}(),
					},
					Logger: NewTestLogger(),
					ID:     [2]byte{0, 1},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ThreadedNewsMgr: func() *hltest.MockThreadNewsMgr {
							m := hltest.MockThreadNewsMgr{}
							m.On("DeleteArticle", []string{"test"}, uint32(1), false).Return(errors.New("write error"))
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranDelNewsArt, [2]byte{0, 1},
					hotline.NewField(hotline.FieldNewsPath,
						[]byte{
							0, 1,
							0, 0,
							4,
							0x74, 0x65, 0x73, 0x74,
						},
					),
					hotline.NewField(hotline.FieldNewsArtID, []byte{0, 0, 0, 1}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID:  [2]byte{0, 1},
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("Error deleting news article.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleDelNewsArt(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

// When the news path is empty, the handler should return an error reply rather
// than silently discarding the request.
func TestHandleDelNewsItem_emptyPathReturnsError(t *testing.T) {
	cc := &hotline.ClientConn{
		ID:     [2]byte{0, 1},
		Logger: NewTestLogger(),
		Server: &hotline.Server{
			TextDecoder: charmap.Macintosh.NewDecoder(),
			TextEncoder: charmap.Macintosh.NewEncoder(),
		},
	}
	tr := hotline.NewTransaction(hotline.TranDelNewsItem, [2]byte{})

	gotRes := HandleDelNewsItem(cc, &tr)

	wantRes := []hotline.Transaction{
		{
			ClientID:  [2]byte{0, 1},
			IsReply:   0x01,
			ErrorCode: [4]byte{0, 0, 0, 1},
			Fields: []hotline.Field{
				hotline.NewField(hotline.FieldError, []byte(ErrMsgDeleteNewsItem)),
			},
		},
	}
	TranAssertEqual(t, wantRes, gotRes)
}

func TestHandleDelNewsItem(t *testing.T) {
	type args struct {
		cc *hotline.ClientConn
		t  hotline.Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []hotline.Transaction
	}{
		{
			name: "when user does not have permission to delete a news category",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: hotline.AccessBitmap{},
					},
					ID: [2]byte{0, 1},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ThreadedNewsMgr: func() *hltest.MockThreadNewsMgr {
							m := hltest.MockThreadNewsMgr{}
							m.On("NewsItem", []string{"test"}).Return(hotline.NewsCategoryListData15{
								Type: hotline.NewsCategory,
							})
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranDelNewsItem, [2]byte{},
					hotline.NewField(hotline.FieldNewsPath,
						[]byte{
							0, 1,
							0, 0,
							4,
							0x74, 0x65, 0x73, 0x74,
						},
					),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID:  [2]byte{0, 1},
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to delete news categories.")),
					},
				},
			},
		},
		{
			name: "when user does not have permission to delete a news folder",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: hotline.AccessBitmap{},
					},
					ID: [2]byte{0, 1},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ThreadedNewsMgr: func() *hltest.MockThreadNewsMgr {
							m := hltest.MockThreadNewsMgr{}
							m.On("NewsItem", []string{"test"}).Return(hotline.NewsCategoryListData15{
								Type: hotline.NewsBundle,
							})
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranDelNewsItem, [2]byte{},
					hotline.NewField(hotline.FieldNewsPath,
						[]byte{
							0, 1,
							0, 0,
							4,
							0x74, 0x65, 0x73, 0x74,
						},
					),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID:  [2]byte{0, 1},
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to delete news folders.")),
					},
				},
			},
		},
		{
			name: "when user deletes a news folder",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessNewsDeleteFldr)
							return bits
						}(),
					},
					ID: [2]byte{0, 1},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ThreadedNewsMgr: func() *hltest.MockThreadNewsMgr {
							m := hltest.MockThreadNewsMgr{}
							m.On("NewsItem", []string{"test"}).Return(hotline.NewsCategoryListData15{Type: hotline.NewsBundle})
							m.On("DeleteNewsItem", []string{"test"}).Return(nil)
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranDelNewsItem, [2]byte{},
					hotline.NewField(hotline.FieldNewsPath,
						[]byte{
							0, 1,
							0, 0,
							4,
							0x74, 0x65, 0x73, 0x74,
						},
					),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields:   []hotline.Field{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleDelNewsItem(tt.args.cc, &tt.args.t)

			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleTranOldPostNews(t *testing.T) {
	type args struct {
		cc *hotline.ClientConn
		t  hotline.Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []hotline.Transaction
	}{
		{
			name: "when user does not have required permission",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: hotline.AccessBitmap{},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranOldPostNews, [2]byte{0, 1},
					hotline.NewField(hotline.FieldData, []byte("hai")),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to post news.")),
					},
				},
			},
		},
		{
			name: "when user posts news update",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessNewsPostArt)
							return bits
						}(),
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						Config: hotline.Config{
							NewsDateFormat: "",
						},
						ClientMgr: func() *hltest.MockClientMgr {
							m := hltest.MockClientMgr{}
							m.On("List").Return([]*hotline.ClientConn{})
							return &m
						}(),
						MessageBoard: func() *mockReadWriteSeeker {
							m := mockReadWriteSeeker{}
							m.On("Seek", int64(0), 0).Return(int64(0), nil)
							m.On("Read", mock.AnythingOfType("[]uint8")).Run(func(args mock.Arguments) {
								arg := args.Get(0).([]uint8)
								copy(arg, "TEST")
							}).Return(4, io.EOF)
							m.On("Write", mock.AnythingOfType("[]uint8")).Return(3, nil)
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranOldPostNews, [2]byte{0, 1},
					hotline.NewField(hotline.FieldData, []byte("hai")),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply: 0x01,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleTranOldPostNews(tt.args.cc, &tt.args.t)

			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleGetNewsArtData(t *testing.T) {
	type args struct {
		cc *hotline.ClientConn
		t  hotline.Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []hotline.Transaction
	}{
		{
			name: "when user does not have required permission",
			args: args{
				cc: &hotline.ClientConn{Account: &hotline.Account{}},
				t: hotline.NewTransaction(
					hotline.TranGetNewsArtData, [2]byte{0, 1},
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to read news.")),
					},
				},
			},
		},
		{
			name: "when user has required permission",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessNewsReadArt)
							return bits
						}(),
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ThreadedNewsMgr: func() *hltest.MockThreadNewsMgr {
							m := hltest.MockThreadNewsMgr{}
							m.On("GetArticle", []string{"Example Category"}, uint32(1)).Return(&hotline.NewsArtData{
								Title:         "title",
								Poster:        "poster",
								Date:          [8]byte{},
								PrevArt:       [4]byte{0, 0, 0, 1},
								NextArt:       [4]byte{0, 0, 0, 2},
								ParentArt:     [4]byte{0, 0, 0, 3},
								FirstChildArt: [4]byte{0, 0, 0, 4},
								DataFlav:      []byte("text/plain"),
								Data:          "article data",
							})
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranGetNewsArtData, [2]byte{0, 1},
					hotline.NewField(hotline.FieldNewsPath, []byte{
						// Example Category
						0x00, 0x01, 0x00, 0x00, 0x10, 0x45, 0x78, 0x61, 0x6d, 0x70, 0x6c, 0x65, 0x20, 0x43, 0x61, 0x74, 0x65, 0x67, 0x6f, 0x72, 0x79,
					}),
					hotline.NewField(hotline.FieldNewsArtID, []byte{0, 1}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply: 1,
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldNewsArtTitle, []byte("title")),
						hotline.NewField(hotline.FieldNewsArtPoster, []byte("poster")),
						hotline.NewField(hotline.FieldNewsArtDate, []byte{0, 0, 0, 0, 0, 0, 0, 0}),
						hotline.NewField(hotline.FieldNewsArtPrevArt, []byte{0, 0, 0, 1}),
						hotline.NewField(hotline.FieldNewsArtNextArt, []byte{0, 0, 0, 2}),
						hotline.NewField(hotline.FieldNewsArtParentArt, []byte{0, 0, 0, 3}),
						hotline.NewField(hotline.FieldNewsArt1stChildArt, []byte{0, 0, 0, 4}),
						hotline.NewField(hotline.FieldNewsArtDataFlav, []byte("text/plain")),
						hotline.NewField(hotline.FieldNewsArtData, []byte("article data")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleGetNewsArtData(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleGetNewsArtNameList(t *testing.T) {
	type args struct {
		cc *hotline.ClientConn
		t  hotline.Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []hotline.Transaction
	}{
		{
			name: "when user does not have required permission",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							return bits
						}(),
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						//Accounts: map[string]*Account{},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranGetNewsArtNameList, [2]byte{0, 1},
				),
			},
			wantRes: []hotline.Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      [2]byte{0, 0},
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to read news.")),
					},
				},
			},
		},
		{
			name: "when user has required access",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessNewsReadArt)
							return bits
						}(),
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ThreadedNewsMgr: func() *hltest.MockThreadNewsMgr {
							m := hltest.MockThreadNewsMgr{}
							m.On("ListArticles", []string{"Example Category"}).Return(hotline.NewsArtListData{
								Name:        []byte{},
								Description: []byte{},
								NewsArtList: []byte{},
							}, nil)
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranGetNewsArtNameList,
					[2]byte{0, 1},
					hotline.NewField(hotline.FieldNewsPath, []byte{
						0x00, 0x01, 0x00, 0x00, 0x10, 0x45, 0x78, 0x61, 0x6d, 0x70, 0x6c, 0x65, 0x20, 0x43, 0x61, 0x74, 0x65, 0x67, 0x6f, 0x72, 0x79,
					}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply: 0x01,
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldNewsArtListData, []byte{
							0x00, 0x00, 0x00, 0x00,
							0x00, 0x00, 0x00, 0x00,
							0x00,
							0x00,
						}),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleGetNewsArtNameList(tt.args.cc, &tt.args.t)

			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleNewNewsFldr(t *testing.T) {
	type args struct {
		cc *hotline.ClientConn
		t  hotline.Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []hotline.Transaction
	}{
		{
			name: "when user does not have required permission",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							return bits
						}(),
					},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						//Accounts: map[string]*Account{},
					},
				},
				t: hotline.NewTransaction(
					hotline.TranGetNewsArtNameList, [2]byte{0, 1},
				),
			},
			wantRes: []hotline.Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      [2]byte{0, 0},
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to create news folders.")),
					},
				},
			},
		},
		{
			name: "with a valid request",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessNewsCreateFldr)
							return bits
						}(),
					},
					Logger: NewTestLogger(),
					ID:     [2]byte{0, 1},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ThreadedNewsMgr: func() *hltest.MockThreadNewsMgr {
							m := hltest.MockThreadNewsMgr{}
							m.On("CreateGrouping", []string{"test"}, "testFolder", hotline.NewsBundle).Return(nil)
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranGetNewsArtNameList, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testFolder")),
					hotline.NewField(hotline.FieldNewsPath,
						[]byte{
							0, 1,
							0, 0,
							4,
							0x74, 0x65, 0x73, 0x74,
						},
					),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields:   []hotline.Field{},
				},
			},
		},
		{
			name: "when CreateGrouping returns an error",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessNewsCreateFldr)
							return bits
						}(),
					},
					Logger: NewTestLogger(),
					ID:     [2]byte{0, 1},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ThreadedNewsMgr: func() *hltest.MockThreadNewsMgr {
							m := hltest.MockThreadNewsMgr{}
							m.On("CreateGrouping", []string{"test"}, "testFolder", hotline.NewsBundle).Return(errors.New("write error"))
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranGetNewsArtNameList, [2]byte{0, 1},
					hotline.NewField(hotline.FieldFileName, []byte("testFolder")),
					hotline.NewField(hotline.FieldNewsPath,
						[]byte{
							0, 1,
							0, 0,
							4,
							0x74, 0x65, 0x73, 0x74,
						},
					),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID:  [2]byte{0, 1},
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("Error creating news folder.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleNewNewsFldr(tt.args.cc, &tt.args.t)

			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandlePostNewsArt(t *testing.T) {
	type args struct {
		cc *hotline.ClientConn
		t  hotline.Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []hotline.Transaction
	}{
		{
			name: "without required permission",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							return bits
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranPostNewsArt,
					[2]byte{0, 0},
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("You are not allowed to post news articles.")),
					},
				},
			},
		},
		{
			name: "with required permission",
			args: args{
				cc: &hotline.ClientConn{
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ThreadedNewsMgr: func() *hltest.MockThreadNewsMgr {
							m := hltest.MockThreadNewsMgr{}
							m.On("PostArticle", []string{"www"}, uint32(0), mock.AnythingOfType("hotline.NewsArtData")).Return(nil)
							return &m
						}(),
					},
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessNewsPostArt)
							return bits
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranPostNewsArt,
					[2]byte{0, 0},
					hotline.NewField(hotline.FieldNewsPath, []byte{0x00, 0x01, 0x00, 0x00, 0x03, 0x77, 0x77, 0x77}),
					hotline.NewField(hotline.FieldNewsArtID, []byte{0x00, 0x00, 0x00, 0x00}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 0},
					Fields:    []hotline.Field{},
				},
			},
		},
		{
			name: "when PostArticle returns an error",
			args: args{
				cc: &hotline.ClientConn{
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ThreadedNewsMgr: func() *hltest.MockThreadNewsMgr {
							m := hltest.MockThreadNewsMgr{}
							m.On("PostArticle", []string{"www"}, uint32(0), mock.AnythingOfType("hotline.NewsArtData")).Return(errors.New("write error"))
							return &m
						}(),
					},
					Logger: NewTestLogger(),
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessNewsPostArt)
							return bits
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranPostNewsArt,
					[2]byte{0, 0},
					hotline.NewField(hotline.FieldNewsPath, []byte{0x00, 0x01, 0x00, 0x00, 0x03, 0x77, 0x77, 0x77}),
					hotline.NewField(hotline.FieldNewsArtID, []byte{0x00, 0x00, 0x00, 0x00}),
				),
			},
			wantRes: []hotline.Transaction{
				{
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte("Error posting news article.")),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			TranAssertEqual(t, tt.wantRes, HandlePostNewsArt(tt.args.cc, &tt.args.t))
		})
	}
}

func TestHandleNewNewsCat(t *testing.T) {
	type args struct {
		cc *hotline.ClientConn
		t  hotline.Transaction
	}
	tests := []struct {
		name    string
		args    args
		wantRes []hotline.Transaction
	}{
		{
			name: "when user does not have required permission",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							return bits
						}(),
					},
					Server: &hotline.Server{TextDecoder: charmap.Macintosh.NewDecoder(), TextEncoder: charmap.Macintosh.NewEncoder()},
				},
				t: hotline.NewTransaction(
					hotline.TranNewNewsCat, [2]byte{0, 1},
				),
			},
			wantRes: []hotline.Transaction{
				{
					Flags:     0x00,
					IsReply:   0x01,
					Type:      [2]byte{0, 0},
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte(ErrMsgNotAllowedCreateNewsCategories)),
					},
				},
			},
		},
		{
			name: "when CreateGrouping returns an error",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessNewsCreateCat)
							return bits
						}(),
					},
					Logger: NewTestLogger(),
					ID:     [2]byte{0, 1},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ThreadedNewsMgr: func() *hltest.MockThreadNewsMgr {
							m := hltest.MockThreadNewsMgr{}
							m.On("CreateGrouping", []string{"test"}, "TestCat", hotline.NewsCategory).Return(errors.New("write error"))
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranNewNewsCat, [2]byte{0, 1},
					hotline.NewField(hotline.FieldNewsCatName, []byte("TestCat")),
					hotline.NewField(hotline.FieldNewsPath,
						[]byte{
							0, 1,
							0, 0,
							4,
							0x74, 0x65, 0x73, 0x74,
						},
					),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID:  [2]byte{0, 1},
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte(ErrMsgCreateNewsCategory)),
					},
				},
			},
		},
		{
			name: "with a valid request",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessNewsCreateCat)
							return bits
						}(),
					},
					Logger: NewTestLogger(),
					ID:     [2]byte{0, 1},
					Server: &hotline.Server{
						TextDecoder: charmap.Macintosh.NewDecoder(),
						TextEncoder: charmap.Macintosh.NewEncoder(),
						ThreadedNewsMgr: func() *hltest.MockThreadNewsMgr {
							m := hltest.MockThreadNewsMgr{}
							m.On("CreateGrouping", []string{"test"}, "TestCat", hotline.NewsCategory).Return(nil)
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(
					hotline.TranNewNewsCat, [2]byte{0, 1},
					hotline.NewField(hotline.FieldNewsCatName, []byte("TestCat")),
					hotline.NewField(hotline.FieldNewsPath,
						[]byte{
							0, 1,
							0, 0,
							4,
							0x74, 0x65, 0x73, 0x74,
						},
					),
				),
			},
			wantRes: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields:   []hotline.Field{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes := HandleNewNewsCat(tt.args.cc, &tt.args.t)

			TranAssertEqual(t, tt.wantRes, gotRes)
		})
	}
}

func TestHandleGetNewsCatNameList(t *testing.T) {
	type args struct {
		cc *hotline.ClientConn
		t  hotline.Transaction
	}
	tests := []struct {
		name string
		args args
		want []hotline.Transaction
	}{
		{
			name: "without news read access returns error",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{Access: hotline.AccessBitmap{}},
					ID:      [2]byte{0, 1},
				},
				t: hotline.NewTransaction(hotline.TranGetNewsCatNameList, [2]byte{0, 1}),
			},
			want: []hotline.Transaction{
				{
					ClientID:  [2]byte{0, 1},
					IsReply:   0x01,
					ErrorCode: [4]byte{0, 0, 0, 1},
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldError, []byte(ErrMsgNotAllowedReadNews)),
					},
				},
			},
		},
		{
			name: "with access returns news categories",
			args: args{
				cc: &hotline.ClientConn{
					Account: &hotline.Account{
						Access: func() hotline.AccessBitmap {
							var bits hotline.AccessBitmap
							bits.Set(hotline.AccessNewsReadArt)
							return bits
						}(),
					},
					ID:     [2]byte{0, 1},
					Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
					Server: &hotline.Server{
						ThreadedNewsMgr: func() *hltest.MockThreadNewsMgr {
							m := hltest.MockThreadNewsMgr{}
							m.On("GetCategories", []string{}).Return([]hotline.NewsCategoryListData15{
								{
									Type: hotline.NewsBundle,
									Name: "Test Bundle",
								},
							})
							return &m
						}(),
					},
				},
				t: hotline.NewTransaction(hotline.TranGetNewsCatNameList, [2]byte{0, 1}),
			},
			want: []hotline.Transaction{
				{
					ClientID: [2]byte{0, 1},
					IsReply:  0x01,
					Fields: []hotline.Field{
						hotline.NewField(hotline.FieldNewsCatListData15, []byte{
							0x00, 0x02, // Type: bundle
							0x00, 0x00, // Count: 0 articles+subcats
							0x0b,                                                             // Name length: 11
							0x54, 0x65, 0x73, 0x74, 0x20, 0x42, 0x75, 0x6e, 0x64, 0x6c, 0x65, // "Test Bundle"
						}),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HandleGetNewsCatNameList(tt.args.cc, &tt.args.t)
			TranAssertEqual(t, tt.want, got)
		})
	}
}
