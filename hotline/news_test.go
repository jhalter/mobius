package hotline

import (
	"github.com/stretchr/testify/assert"
	"io"
	"testing"
)

func TestNewsCategoryListData15_MarshalBinary(t *testing.T) {
	type fields struct {
		Type     [2]byte
		Name     string
		Articles map[uint32]*NewsArtData
		SubCats  map[string]NewsCategoryListData15
		Count    []byte
		AddSN    [4]byte
		DeleteSN [4]byte
		GUID     [16]byte
	}
	tests := []struct {
		name     string
		fields   fields
		wantData []byte
		wantErr  bool
	}{
		{
			name: "returns expected bytes when type is a bundle",
			fields: fields{
				Type: [2]byte{0x00, 0x02},
				Articles: map[uint32]*NewsArtData{
					uint32(1): {
						Title:  "",
						Poster: "",
						Data:   "",
					},
				},
				Name: "foo",
			},
			wantData: []byte{
				0x00, 0x02,
				0x00, 0x01,
				0x03,
				0x66, 0x6f, 0x6f,
			},
			wantErr: false,
		},
		{
			name: "returns expected bytes when type is a category",
			fields: fields{
				Type: [2]byte{0x00, 0x03},
				Articles: map[uint32]*NewsArtData{
					uint32(1): {
						Title:  "",
						Poster: "",
						Data:   "",
					},
				},
				Name: "foo",
			},
			wantData: []byte{
				0x00, 0x03,
				0x00, 0x01,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
				0x03,
				0x66, 0x6f, 0x6f,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newscat := &NewsCategoryListData15{
				Type:     tt.fields.Type,
				Name:     tt.fields.Name,
				Articles: tt.fields.Articles,
				SubCats:  tt.fields.SubCats,
				AddSN:    tt.fields.AddSN,
				DeleteSN: tt.fields.DeleteSN,
				GUID:     tt.fields.GUID,
			}
			gotData, err := io.ReadAll(newscat)
			if newscat.Type == [2]byte{0, 3} {
				// zero out the random GUID before comparison
				for i := 4; i < 20; i++ {
					gotData[i] = 0
				}
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalBinary() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !assert.Equal(t, tt.wantData, gotData) {
				t.Errorf("MarshalBinary() gotData = %v, want %v", gotData, tt.wantData)
			}
		})
	}
}

func TestNewsCategoryListData15_GetNewsArtListData(t *testing.T) {
	tests := []struct {
		name     string
		newscat  NewsCategoryListData15
		wantData NewsArtListData
		wantErr  bool
	}{
		{
			name: "empty articles",
			newscat: NewsCategoryListData15{
				Articles: map[uint32]*NewsArtData{},
			},
			wantData: NewsArtListData{
				Count:       0,
				Name:        []byte{},
				Description: []byte{},
				NewsArtList: []byte{},
			},
			wantErr: false,
		},
		{
			name: "single article",
			newscat: NewsCategoryListData15{
				Articles: map[uint32]*NewsArtData{
					1: {
						Title:  "Test Title",
						Poster: "Test Poster",
						Date:   [8]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07},
						Data:   "Test content",
					},
				},
			},
			wantData: NewsArtListData{
				Count:       1,
				Name:        []byte{},
				Description: []byte{},
			},
			wantErr: false,
		},
		{
			name: "multiple articles",
			newscat: NewsCategoryListData15{
				Articles: map[uint32]*NewsArtData{
					2: {
						Title:  "Second Article",
						Poster: "Author2",
						Date:   [8]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x08},
						Data:   "Second content",
					},
					1: {
						Title:  "First Article",
						Poster: "Author1",
						Date:   [8]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07},
						Data:   "First content",
					},
				},
			},
			wantData: NewsArtListData{
				Count:       2,
				Name:        []byte{},
				Description: []byte{},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotData, err := tt.newscat.GetNewsArtListData()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetNewsArtListData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.wantData.Count, gotData.Count)
			assert.Equal(t, tt.wantData.Name, gotData.Name)
			assert.Equal(t, tt.wantData.Description, gotData.Description)
			if tt.wantData.Count > 0 {
				assert.NotEmpty(t, gotData.NewsArtList)
			}
		})
	}
}

func TestNewsArtData_DataSize(t *testing.T) {
	tests := []struct {
		name string
		art  NewsArtData
		want [2]byte
	}{
		{
			name: "empty data",
			art:  NewsArtData{Data: ""},
			want: [2]byte{0x00, 0x00},
		},
		{
			name: "short data",
			art:  NewsArtData{Data: "hello"},
			want: [2]byte{0x00, 0x05},
		},
		{
			name: "longer data",
			art:  NewsArtData{Data: "This is a longer test message with more content"},
			want: [2]byte{0x00, 0x2F}, // 47 bytes
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.art.DataSize()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewsArtListData_Read(t *testing.T) {
	tests := []struct {
		name       string
		nald       NewsArtListData
		bufferSize int
		wantN      int
		wantErr    bool
	}{
		{
			name: "empty data",
			nald: NewsArtListData{
				ID:          [4]byte{0x00, 0x01, 0x02, 0x03},
				Name:        []byte("test"),
				Description: []byte("desc"),
				NewsArtList: []byte{},
				Count:       0,
			},
			bufferSize: 100,
			wantN:      18, // 4 (ID) + 4 (count) + 1 (name len) + 4 (name) + 1 (desc len) + 4 (desc) + 0 (news art list)
			wantErr:    false,
		},
		{
			name: "with article list",
			nald: NewsArtListData{
				ID:          [4]byte{0x00, 0x01, 0x02, 0x03},
				Name:        []byte("test"),
				Description: []byte("desc"),
				NewsArtList: []byte{0x01, 0x02, 0x03},
				Count:       1,
			},
			bufferSize: 100,
			wantN:      21, // 4 (ID) + 4 (count) + 1 (name len) + 4 (name) + 1 (desc len) + 4 (desc) + 3 (news art list)
			wantErr:    false,
		},
		{
			name: "small buffer",
			nald: NewsArtListData{
				ID:          [4]byte{0x00, 0x01, 0x02, 0x03},
				Name:        []byte("test"),
				Description: []byte("desc"),
				Count:       0,
			},
			bufferSize: 5,
			wantN:      5,
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := make([]byte, tt.bufferSize)
			gotN, err := tt.nald.Read(p)
			if (err != nil) != tt.wantErr {
				t.Errorf("Read() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.wantN, gotN)
		})
	}
}

func TestNewsArtList_Read(t *testing.T) {
	tests := []struct {
		name       string
		nal        NewsArtList
		bufferSize int
		wantN      int
		wantErr    bool
	}{
		{
			name: "basic article",
			nal: NewsArtList{
				ID:          [4]byte{0x00, 0x01, 0x02, 0x03},
				TimeStamp:   [8]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07},
				ParentID:    [4]byte{0x00, 0x00, 0x00, 0x00},
				Flags:       [4]byte{0x00, 0x00, 0x00, 0x00},
				Title:       []byte("Test Title"),
				Poster:      []byte("Test Poster"),
				ArticleSize: [2]byte{0x00, 0x0A},
			},
			bufferSize: 100,
			wantN:      58, // 4 (ID) + 8 (timestamp) + 4 (parent) + 4 (flags) + 2 (flavor count) + 1 (title len) + 10 (title) + 1 (poster len) + 11 (poster) + 1 (flavor len) + 10 (flavor) + 2 (article size)
			wantErr:    false,
		},
		{
			name: "small buffer",
			nal: NewsArtList{
				ID:        [4]byte{0x00, 0x01, 0x02, 0x03},
				TimeStamp: [8]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07},
				Title:     []byte("Test"),
				Poster:    []byte("Author"),
			},
			bufferSize: 10,
			wantN:      10,
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := make([]byte, tt.bufferSize)
			gotN, err := tt.nal.Read(p)
			if (err != nil) != tt.wantErr {
				t.Errorf("Read() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.wantN, gotN)
		})
	}
}

func TestNewsPathScanner(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		wantAdvance int
		wantToken   []byte
		wantErr     bool
	}{
		{
			name:        "insufficient data",
			data:        []byte{0x00, 0x01},
			wantAdvance: 0,
			wantToken:   nil,
			wantErr:     false,
		},
		{
			name:        "valid token",
			data:        []byte{0x00, 0x01, 0x04, 0x74, 0x65, 0x73, 0x74}, // length 4, "test"
			wantAdvance: 7,
			wantToken:   []byte("test"),
			wantErr:     false,
		},
		{
			name:        "zero length token",
			data:        []byte{0x00, 0x01, 0x00},
			wantAdvance: 3,
			wantToken:   []byte{},
			wantErr:     false,
		},
		{
			name:        "single character token",
			data:        []byte{0x00, 0x01, 0x01, 0x61}, // length 1, "a"
			wantAdvance: 4,
			wantToken:   []byte("a"),
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAdvance, gotToken, err := newsPathScanner(tt.data, false)
			if (err != nil) != tt.wantErr {
				t.Errorf("newsPathScanner() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.wantAdvance, gotAdvance)
			assert.Equal(t, tt.wantToken, gotToken)
		})
	}
}
