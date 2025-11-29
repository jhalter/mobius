package hotline

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestNewsArtListData_Write(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		wantID      [4]byte
		wantCount   int
		wantName    []byte
		wantDesc    []byte
		wantArtList []byte
		wantN       int
		wantErr     bool
	}{
		{
			name: "basic data",
			input: []byte{
				0x00, 0x01, 0x02, 0x03, // ID
				0x00, 0x00, 0x00, 0x05, // Count: 5
				0x04,                   // Name length: 4
				0x74, 0x65, 0x73, 0x74, // Name: "test"
				0x04,                   // Description length: 4
				0x64, 0x65, 0x73, 0x63, // Description: "desc"
			},
			wantID:      [4]byte{0x00, 0x01, 0x02, 0x03},
			wantCount:   5,
			wantName:    []byte("test"),
			wantDesc:    []byte("desc"),
			wantArtList: []byte{},
			wantN:       18,
			wantErr:     false,
		},
		{
			name: "with article list",
			input: []byte{
				0x00, 0x01, 0x02, 0x03, // ID
				0x00, 0x00, 0x00, 0x01, // Count: 1
				0x04,                   // Name length: 4
				0x74, 0x65, 0x73, 0x74, // Name: "test"
				0x04,                   // Description length: 4
				0x64, 0x65, 0x73, 0x63, // Description: "desc"
				0xAA, 0xBB, 0xCC, // NewsArtList data
			},
			wantID:      [4]byte{0x00, 0x01, 0x02, 0x03},
			wantCount:   1,
			wantName:    []byte("test"),
			wantDesc:    []byte("desc"),
			wantArtList: []byte{0xAA, 0xBB, 0xCC},
			wantN:       21,
			wantErr:     false,
		},
		{
			name: "empty name and description",
			input: []byte{
				0xFF, 0xFE, 0xFD, 0xFC, // ID
				0x00, 0x00, 0x00, 0x00, // Count: 0
				0x00, // Name length: 0
				0x00, // Description length: 0
			},
			wantID:      [4]byte{0xFF, 0xFE, 0xFD, 0xFC},
			wantCount:   0,
			wantName:    []byte{},
			wantDesc:    []byte{},
			wantArtList: []byte{},
			wantN:       10,
			wantErr:     false,
		},
		{
			name: "long name and description",
			input: []byte{
				0x00, 0x00, 0x00, 0x00, // ID
				0x00, 0x00, 0x00, 0x0A, // Count: 10
				0x0A,                                                       // Name length: 10
				0x4C, 0x6F, 0x6E, 0x67, 0x65, 0x72, 0x4E, 0x61, 0x6D, 0x65, // Name: "LongerName"
				0x0B,                                                             // Description length: 11
				0x44, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6F, 0x6E, // Description: "Description"
			},
			wantID:      [4]byte{0x00, 0x00, 0x00, 0x00},
			wantCount:   10,
			wantName:    []byte("LongerName"),
			wantDesc:    []byte("Description"),
			wantArtList: []byte{},
			wantN:       31, // 4 (ID) + 4 (count) + 1 (name len) + 10 (name) + 1 (desc len) + 11 (desc)
			wantErr:     false,
		},
		{
			name: "with large article list",
			input: []byte{
				0x12, 0x34, 0x56, 0x78, // ID
				0x00, 0x00, 0x00, 0x03, // Count: 3
				0x02,       // Name length: 2
				0x41, 0x42, // Name: "AB"
				0x02,       // Description length: 2
				0x43, 0x44, // Description: "CD"
				0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, // NewsArtList: 10 bytes
			},
			wantID:      [4]byte{0x12, 0x34, 0x56, 0x78},
			wantCount:   3,
			wantName:    []byte("AB"),
			wantDesc:    []byte("CD"),
			wantArtList: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A},
			wantN:       24,
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nald := &NewsArtListData{}
			gotN, err := nald.Write(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Write() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.wantN, gotN)
			assert.Equal(t, tt.wantID, nald.ID)
			assert.Equal(t, tt.wantCount, nald.Count)
			assert.Equal(t, tt.wantName, nald.Name)
			assert.Equal(t, tt.wantDesc, nald.Description)
			assert.Equal(t, tt.wantArtList, nald.NewsArtList)
		})
	}
}

func TestNewsArtListData_Write_PartialData(t *testing.T) {
	tests := []struct {
		name        string
		chunks      [][]byte
		wantID      [4]byte
		wantCount   int
		wantName    []byte
		wantDesc    []byte
		wantArtList []byte
		finalBytes  int
	}{
		{
			name: "split across ID boundary",
			chunks: [][]byte{
				{0x00, 0x01},             // First 2 bytes of ID
				{0x02, 0x03},             // Last 2 bytes of ID
				{0x00, 0x00, 0x00, 0x02}, // Count: 2
				{0x03},                   // Name length: 3
				{0x66, 0x6f, 0x6f},       // Name: "foo"
				{0x03},                   // Description length: 3
				{0x62, 0x61, 0x72},       // Description: "bar"
			},
			wantID:      [4]byte{0x00, 0x01, 0x02, 0x03},
			wantCount:   2,
			wantName:    []byte("foo"),
			wantDesc:    []byte("bar"),
			wantArtList: []byte{},
			finalBytes:  16, // 2 + 2 + 4 + 1 + 3 + 1 + 3
		},
		{
			name: "split across name",
			chunks: [][]byte{
				{0x00, 0x00, 0x00, 0x00}, // ID
				{0x00, 0x00, 0x00, 0x01}, // Count: 1
				{0x05},                   // Name length: 5
				{0x68, 0x65},             // "he"
				{0x6c, 0x6c, 0x6f},       // "llo"
				{0x00},                   // Description length: 0
			},
			wantID:      [4]byte{0x00, 0x00, 0x00, 0x00},
			wantCount:   1,
			wantName:    []byte("hello"),
			wantDesc:    []byte{},
			wantArtList: []byte{},
			finalBytes:  15,
		},
		{
			name: "split with article list",
			chunks: [][]byte{
				{0xAA, 0xBB, 0xCC, 0xDD}, // ID
				{0x00, 0x00, 0x00, 0x05}, // Count: 5
				{0x01, 0x41},             // Name length: 1, Name: "A"
				{0x01, 0x42},             // Description length: 1, Description: "B"
				{0x11, 0x22},             // Article list part 1
				{0x33, 0x44, 0x55},       // Article list part 2
			},
			wantID:      [4]byte{0xAA, 0xBB, 0xCC, 0xDD},
			wantCount:   5,
			wantName:    []byte("A"),
			wantDesc:    []byte("B"),
			wantArtList: []byte{0x11, 0x22, 0x33, 0x44, 0x55},
			finalBytes:  17,
		},
		{
			name: "single byte chunks",
			chunks: [][]byte{
				{0x01}, {0x02}, {0x03}, {0x04}, // ID
				{0x00}, {0x00}, {0x00}, {0x00}, // Count: 0
				{0x02},         // Name length: 2
				{0x41}, {0x42}, // Name: "AB"
				{0x00}, // Description length: 0
			},
			wantID:      [4]byte{0x01, 0x02, 0x03, 0x04},
			wantCount:   0,
			wantName:    []byte("AB"),
			wantDesc:    []byte{},
			wantArtList: []byte{},
			finalBytes:  12,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nald := &NewsArtListData{}
			totalBytes := 0
			for _, chunk := range tt.chunks {
				n, err := nald.Write(chunk)
				assert.NoError(t, err)
				assert.Equal(t, len(chunk), n)
				totalBytes += n
			}
			assert.Equal(t, tt.finalBytes, totalBytes)
			assert.Equal(t, tt.wantID, nald.ID)
			assert.Equal(t, tt.wantCount, nald.Count)
			assert.Equal(t, tt.wantName, nald.Name)
			assert.Equal(t, tt.wantDesc, nald.Description)
			assert.Equal(t, tt.wantArtList, nald.NewsArtList)
		})
	}
}

func TestNewsArtListData_WriteRead_RoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		nald    NewsArtListData
		wantErr bool
	}{
		{
			name: "basic round trip",
			nald: NewsArtListData{
				ID:          [4]byte{0x00, 0x01, 0x02, 0x03},
				Name:        []byte("Test Name"),
				Description: []byte("Test Description"),
				NewsArtList: []byte{},
				Count:       0,
			},
			wantErr: false,
		},
		{
			name: "with article list",
			nald: NewsArtListData{
				ID:          [4]byte{0xFF, 0xEE, 0xDD, 0xCC},
				Name:        []byte("Articles"),
				Description: []byte("Article Description"),
				NewsArtList: []byte{0x01, 0x02, 0x03, 0x04, 0x05},
				Count:       5,
			},
			wantErr: false,
		},
		{
			name: "empty fields",
			nald: NewsArtListData{
				ID:          [4]byte{0x00, 0x00, 0x00, 0x00},
				Name:        []byte{},
				Description: []byte{},
				NewsArtList: []byte{},
				Count:       0,
			},
			wantErr: false,
		},
		{
			name: "max length name and description",
			nald: NewsArtListData{
				ID:          [4]byte{0x12, 0x34, 0x56, 0x78},
				Name:        []byte("This is a very long name for testing purposes with lots of characters"),
				Description: []byte("This is an equally long description to ensure we handle variable length fields properly"),
				NewsArtList: []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF},
				Count:       100,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Read the struct into bytes
			data, err := io.ReadAll(&tt.nald)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadAll() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Write the bytes back into a new struct
			result := &NewsArtListData{}
			n, err := result.Write(data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Write() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify the round trip
			assert.Equal(t, len(data), n)
			assert.Equal(t, tt.nald.ID, result.ID)
			assert.Equal(t, tt.nald.Count, result.Count)
			assert.Equal(t, tt.nald.Name, result.Name)
			assert.Equal(t, tt.nald.Description, result.Description)
			assert.Equal(t, tt.nald.NewsArtList, result.NewsArtList)
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

func TestNewsCategoryListData15_Write(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		wantType [2]byte
		wantName string
		wantGUID [16]byte
		wantN    int
		wantErr  bool
	}{
		{
			name: "bundle type with name",
			input: []byte{
				0x00, 0x02, // Type: Bundle
				0x00, 0x01, // Count: 1
				0x03,             // Name length: 3
				0x66, 0x6f, 0x6f, // Name: "foo"
			},
			wantType: [2]byte{0x00, 0x02},
			wantName: "foo",
			wantN:    8, // 2 (type) + 2 (count) + 1 (name len) + 3 (name)
			wantErr:  false,
		},
		{
			name: "category type with GUID, AddSN, DeleteSN",
			input: []byte{
				0x00, 0x03, // Type: Category
				0x00, 0x01, // Count: 1
				0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, // GUID part 1
				0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, // GUID part 2
				0x01, 0x02, 0x03, 0x04, // AddSN
				0x05, 0x06, 0x07, 0x08, // DeleteSN
				0x03,             // Name length: 3
				0x62, 0x61, 0x72, // Name: "bar"
			},
			wantType: [2]byte{0x00, 0x03},
			wantName: "bar",
			wantGUID: [16]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00},
			wantN:    32, // 2 (type) + 2 (count) + 16 (GUID) + 4 (AddSN) + 4 (DeleteSN) + 1 (name len) + 3 (name)
			wantErr:  false,
		},
		{
			name: "empty name",
			input: []byte{
				0x00, 0x02, // Type: Bundle
				0x00, 0x00, // Count: 0
				0x00, // Name length: 0
			},
			wantType: [2]byte{0x00, 0x02},
			wantName: "",
			wantN:    5,
			wantErr:  false,
		},
		{
			name: "long name",
			input: []byte{
				0x00, 0x02, // Type: Bundle
				0x00, 0x05, // Count: 5
				0x0a,                                                       // Name length: 10
				0x4c, 0x6f, 0x6e, 0x67, 0x65, 0x72, 0x4e, 0x61, 0x6d, 0x65, // Name: "LongerName"
			},
			wantType: [2]byte{0x00, 0x02},
			wantName: "LongerName",
			wantN:    15,
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newscat := &NewsCategoryListData15{}
			gotN, err := newscat.Write(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Write() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.wantN, gotN)
			assert.Equal(t, tt.wantType, newscat.Type)
			assert.Equal(t, tt.wantName, newscat.Name)
			if tt.wantType == NewsCategory {
				assert.Equal(t, tt.wantGUID, newscat.GUID)
			}
			assert.NotNil(t, newscat.Articles)
			assert.NotNil(t, newscat.SubCats)
		})
	}
}

func TestNewsCategoryListData15_WriteRead_RoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		newscat NewsCategoryListData15
		wantErr bool
	}{
		{
			name: "bundle round trip",
			newscat: NewsCategoryListData15{
				Type: NewsBundle,
				Name: "Test Bundle",
				Articles: map[uint32]*NewsArtData{
					1: {Title: "Article 1"},
				},
				SubCats: make(map[string]NewsCategoryListData15),
			},
			wantErr: false,
		},
		{
			name: "category round trip",
			newscat: NewsCategoryListData15{
				Type:     NewsCategory,
				Name:     "Test Category",
				GUID:     [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
				AddSN:    [4]byte{0x00, 0x00, 0x00, 0x01},
				DeleteSN: [4]byte{0x00, 0x00, 0x00, 0x02},
				Articles: make(map[uint32]*NewsArtData),
				SubCats: map[string]NewsCategoryListData15{
					"subcat1": {
						Type: NewsBundle,
						Name: "Subcategory",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty name round trip",
			newscat: NewsCategoryListData15{
				Type:     NewsBundle,
				Name:     "",
				Articles: make(map[uint32]*NewsArtData),
				SubCats:  make(map[string]NewsCategoryListData15),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Read the struct into bytes
			data, err := io.ReadAll(&tt.newscat)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadAll() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Write the bytes back into a new struct
			result := &NewsCategoryListData15{}
			n, err := result.Write(data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Write() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify the round trip
			assert.Equal(t, len(data), n)
			assert.Equal(t, tt.newscat.Type, result.Type)
			assert.Equal(t, tt.newscat.Name, result.Name)
			if tt.newscat.Type == NewsCategory {
				assert.Equal(t, tt.newscat.GUID, result.GUID)
				assert.Equal(t, tt.newscat.AddSN, result.AddSN)
				assert.Equal(t, tt.newscat.DeleteSN, result.DeleteSN)
			}
		})
	}
}

func TestNewsCategoryListData15_Write_PartialData(t *testing.T) {
	tests := []struct {
		name       string
		chunks     [][]byte
		wantType   [2]byte
		wantName   string
		finalBytes int
	}{
		{
			name: "split across type boundary",
			chunks: [][]byte{
				{0x00},             // First byte of type
				{0x02},             // Second byte of type
				{0x00, 0x01},       // Count
				{0x03},             // Name length
				{0x66, 0x6f, 0x6f}, // Name: "foo"
			},
			wantType:   [2]byte{0x00, 0x02},
			wantName:   "foo",
			finalBytes: 8, // 1 + 1 + 2 + 1 + 3
		},
		{
			name: "split across name",
			chunks: [][]byte{
				{0x00, 0x02, 0x00, 0x01, 0x05}, // Type, count, name length: 5
				{0x68, 0x65},                   // "he"
				{0x6c, 0x6c, 0x6f},             // "llo"
			},
			wantType:   [2]byte{0x00, 0x02},
			wantName:   "hello",
			finalBytes: 10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newscat := &NewsCategoryListData15{}
			totalBytes := 0
			for _, chunk := range tt.chunks {
				n, err := newscat.Write(chunk)
				assert.NoError(t, err)
				assert.Equal(t, len(chunk), n)
				totalBytes += n
			}
			assert.Equal(t, tt.finalBytes, totalBytes)
			assert.Equal(t, tt.wantType, newscat.Type)
			assert.Equal(t, tt.wantName, newscat.Name)
		})
	}
}
