package mobius

import (
	"fmt"
	"os"
	"path"
	"sync"
	"testing"

	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/assert"
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
				defer func() { _ = os.Remove(tt.fileName) }() // Cleanup the file after the test
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
	defer func() { _ = os.RemoveAll(tmpDir) }() // Clean up the temporary directory.

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

func TestThreadedNewsYAML_CreateGrouping_rollback(t *testing.T) {
	n := &ThreadedNewsYAML{
		ThreadedNews: hotline.ThreadedNews{
			Categories: map[string]hotline.NewsCategoryListData15{
				"Existing": {
					Name:     "Existing",
					Type:     hotline.NewsCategory,
					Articles: make(map[uint32]*hotline.NewsArtData),
					SubCats:  make(map[string]hotline.NewsCategoryListData15),
				},
			},
		},
		filePath: "/nonexistent/dir/ThreadedNews.yaml",
	}

	err := n.CreateGrouping(nil, "NewBundle", hotline.NewsBundle)
	assert.Error(t, err)

	// The new entry should have been rolled back.
	_, exists := n.ThreadedNews.Categories["NewBundle"]
	assert.False(t, exists, "new grouping should be removed on write failure")

	// Existing entry should still be present.
	_, exists = n.ThreadedNews.Categories["Existing"]
	assert.True(t, exists, "existing grouping should be preserved")
}

func TestThreadedNewsYAML_DeleteNewsItem_rollback(t *testing.T) {
	n := &ThreadedNewsYAML{
		ThreadedNews: hotline.ThreadedNews{
			Categories: map[string]hotline.NewsCategoryListData15{
				"ToDelete": {
					Name:     "ToDelete",
					Type:     hotline.NewsCategory,
					Articles: make(map[uint32]*hotline.NewsArtData),
					SubCats:  make(map[string]hotline.NewsCategoryListData15),
				},
			},
		},
		filePath: "/nonexistent/dir/ThreadedNews.yaml",
	}

	err := n.DeleteNewsItem([]string{"ToDelete"})
	assert.Error(t, err)

	// The deleted entry should have been restored.
	cat, exists := n.ThreadedNews.Categories["ToDelete"]
	assert.True(t, exists, "deleted item should be restored on write failure")
	assert.Equal(t, "ToDelete", cat.Name)
}

func TestThreadedNewsYAML_PostArticle_rollback(t *testing.T) {
	// Set up a category with existing articles (mimics the test fixture).
	n := &ThreadedNewsYAML{
		ThreadedNews: hotline.ThreadedNews{
			Categories: map[string]hotline.NewsCategoryListData15{
				"TestCat": {
					Type: hotline.NewsCategory,
					Name: "TestCat",
					Articles: map[uint32]*hotline.NewsArtData{
						1: {
							Title:         "TestArt",
							NextArt:       [4]byte{0, 0, 0, 2},
							FirstChildArt: [4]byte{0, 0, 0, 2},
						},
						2: {
							Title:     "Re: TestArt",
							PrevArt:   [4]byte{0, 0, 0, 1},
							ParentArt: [4]byte{0, 0, 0, 1},
							NextArt:   [4]byte{0, 0, 0, 3},
						},
						3: {
							Title:   "TestArt 2",
							PrevArt: [4]byte{0, 0, 0, 2},
						},
					},
					SubCats: make(map[string]hotline.NewsCategoryListData15),
				},
			},
		},
		filePath: "/nonexistent/dir/ThreadedNews.yaml",
	}

	// Snapshot state before.
	origNextArt3 := n.ThreadedNews.Categories["TestCat"].Articles[3].NextArt

	newArticle := hotline.NewsArtData{
		Title:  "New Article",
		Poster: "tester",
	}

	err := n.PostArticle([]string{"TestCat"}, 0, newArticle)
	assert.Error(t, err)

	cat := n.ThreadedNews.Categories["TestCat"]

	// New article (ID 4) should not exist.
	_, exists := cat.Articles[4]
	assert.False(t, exists, "new article should be removed on write failure")

	// Article 3's NextArt should be restored to its original value.
	assert.Equal(t, origNextArt3, cat.Articles[3].NextArt, "previous article's NextArt should be restored")

	// Should still have exactly 3 articles.
	assert.Len(t, cat.Articles, 3)
}

func TestThreadedNewsYAML_PostArticle_rollback_with_parent(t *testing.T) {
	// Test that FirstChildArt is restored when posting a reply.
	n := &ThreadedNewsYAML{
		ThreadedNews: hotline.ThreadedNews{
			Categories: map[string]hotline.NewsCategoryListData15{
				"TestCat": {
					Type: hotline.NewsCategory,
					Name: "TestCat",
					Articles: map[uint32]*hotline.NewsArtData{
						1: {
							Title: "Parent Article",
							// No FirstChildArt set — this is the first reply.
						},
					},
					SubCats: make(map[string]hotline.NewsCategoryListData15),
				},
			},
		},
		filePath: "/nonexistent/dir/ThreadedNews.yaml",
	}

	newArticle := hotline.NewsArtData{
		Title:  "Reply",
		Poster: "tester",
	}

	err := n.PostArticle([]string{"TestCat"}, 1, newArticle)
	assert.Error(t, err)

	cat := n.ThreadedNews.Categories["TestCat"]

	// FirstChildArt should be restored to zero.
	assert.Equal(t, [4]byte{}, cat.Articles[1].FirstChildArt, "parent's FirstChildArt should be restored")

	// New article should not exist.
	_, exists := cat.Articles[2]
	assert.False(t, exists, "reply article should be removed on write failure")
}

func TestThreadedNewsYAML_DeleteArticle_rollback(t *testing.T) {
	n := &ThreadedNewsYAML{
		ThreadedNews: hotline.ThreadedNews{
			Categories: map[string]hotline.NewsCategoryListData15{
				"TestCat": {
					Type: hotline.NewsCategory,
					Name: "TestCat",
					Articles: map[uint32]*hotline.NewsArtData{
						1: {
							Title: "Article to delete",
						},
					},
					SubCats: make(map[string]hotline.NewsCategoryListData15),
				},
			},
		},
		filePath: "/nonexistent/dir/ThreadedNews.yaml",
	}

	err := n.DeleteArticle([]string{"TestCat"}, 1, false)
	assert.Error(t, err)

	cat := n.ThreadedNews.Categories["TestCat"]

	// Deleted article should be restored.
	art, exists := cat.Articles[1]
	assert.True(t, exists, "deleted article should be restored on write failure")
	assert.Equal(t, "Article to delete", art.Title)
}
