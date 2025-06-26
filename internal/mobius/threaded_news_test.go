package mobius

import (
	"fmt"
	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/assert"
	"os"
	"path"
	"sync"
	"testing"
)

type TestData struct {
	Name  string `yaml:"name"`
	Value int    `yaml:"value"`
}

func TestLoadFromYAMLFile(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		content  string
		wantData TestData
		wantErr  bool
	}{
		{
			name:     "Valid YAML file",
			fileName: "valid.yaml",
			content:  "name: Test\nvalue: 123\n",
			wantData: TestData{Name: "Test", Value: 123},
			wantErr:  false,
		},
		{
			name:     "File not found",
			fileName: "nonexistent.yaml",
			content:  "",
			wantData: TestData{},
			wantErr:  true,
		},
		{
			name:     "Invalid YAML content",
			fileName: "invalid.yaml",
			content:  "name: Test\nvalue: invalid_int\n",
			wantData: TestData{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: Create a temporary file with the provided content if content is not empty
			if tt.content != "" {
				err := os.WriteFile(tt.fileName, []byte(tt.content), 0644)
				assert.NoError(t, err)
				defer os.Remove(tt.fileName) // Cleanup the file after the test
			}

			var data TestData
			err := loadFromYAMLFile(tt.fileName, &data)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantData, data)
			}
		})
	}
}

func TestNewThreadedNewsYAML(t *testing.T) {
	type args struct {
		filePath string
	}
	tests := []struct {
		name    string
		args    args
		want    *ThreadedNewsYAML
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "Valid YAML file",
			args: args{
				filePath: "test/config/ThreadedNews.yaml",
			},
			want: &ThreadedNewsYAML{
				filePath: "test/config/ThreadedNews.yaml",
				ThreadedNews: hotline.ThreadedNews{
					Categories: map[string]hotline.NewsCategoryListData15{
						"TestBundle": {
							Type:     hotline.NewsBundle,
							Name:     "TestBundle",
							Articles: make(map[uint32]*hotline.NewsArtData),
							SubCats: map[string]hotline.NewsCategoryListData15{
								"NestedBundle": {
									Name: "NestedBundle",
									Type: hotline.NewsBundle,
									SubCats: map[string]hotline.NewsCategoryListData15{
										"NestedCat": {
											Name:     "NestedCat",
											Type:     hotline.NewsCategory,
											Articles: make(map[uint32]*hotline.NewsArtData),
											SubCats:  make(map[string]hotline.NewsCategoryListData15),
										},
									},
									Articles: make(map[uint32]*hotline.NewsArtData),
								},
							},
						},
						"TestCat": {
							Type: hotline.NewsCategory,
							Name: "TestCat",
							Articles: map[uint32]*hotline.NewsArtData{
								1: {
									Title:         "TestArt",
									Poster:        "Halcyon 1.9.2",
									Date:          [8]byte{0x07, 0xe4, 0x00, 0x00, 0x00, 0xfe, 0xfc, 0xcc},
									NextArt:       [4]byte{0, 0, 0, 2},
									FirstChildArt: [4]byte{0, 0, 0, 2},
									Data:          "TestArt Body",
								},
								2: {
									Title:     "Re: TestArt",
									Poster:    "Halcyon 1.9.2",
									Date:      [8]byte{0x07, 0xe4, 0x00, 0x00, 0x00, 0xfe, 0xfc, 0xd8},
									PrevArt:   [4]byte{0, 0, 0, 1},
									ParentArt: [4]byte{0, 0, 0, 1},
									NextArt:   [4]byte{0, 0, 0, 3},
									Data:      "I'm a reply",
								},
								3: {
									Title:   "TestArt 2",
									Poster:  "Halcyon 1.9.2",
									Date:    [8]byte{0x07, 0xe4, 0x00, 0x00, 0x00, 0xfe, 0xfd, 0x06},
									PrevArt: [4]byte{0, 0, 0, 2},
									Data:    "Hello world",
								},
							},
							SubCats: make(map[string]hotline.NewsCategoryListData15),
						},
					},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewThreadedNewsYAML(tt.args.filePath)
			if !tt.wantErr(t, err, fmt.Sprintf("NewThreadedNewsYAML(%v)", tt.args.filePath)) {
				return
			}
			assert.Equalf(t, tt.want, got, "NewThreadedNewsYAML(%v)", tt.args.filePath)
		})
	}
}

func TestThreadedNewsYAML_CreateGrouping(t *testing.T) {
	// Create a temporary directory.
	tmpDir, err := os.MkdirTemp("", "createGrouping")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir) // Clean up the temporary directory.

	// Path to the temporary ban file.
	tmpFilePath := path.Join(tmpDir, "ThreadedNews.yaml")

	type fields struct {
		ThreadedNews hotline.ThreadedNews
		filePath     string
	}
	type args struct {
		newsPath []string
		name     string
		t        [2]byte
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "new bundle",
			fields: fields{
				ThreadedNews: hotline.ThreadedNews{
					Categories: map[string]hotline.NewsCategoryListData15{
						"": {
							SubCats: make(map[string]hotline.NewsCategoryListData15),
						},
					},
				},
				filePath: tmpFilePath,
			},
			args: args{
				newsPath: []string{""},
				name:     "new bundle",
				t:        hotline.NewsBundle,
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &ThreadedNewsYAML{
				ThreadedNews: tt.fields.ThreadedNews,
				filePath:     tt.fields.filePath,
				mu:           sync.Mutex{},
			}
			tt.wantErr(t, n.CreateGrouping(tt.args.newsPath, tt.args.name, tt.args.t), fmt.Sprintf("CreateGrouping(%v, %v, %v)", tt.args.newsPath, tt.args.name, tt.args.t))
		})
	}
}
