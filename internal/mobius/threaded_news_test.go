package mobius

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// newTestThreadedNews creates a ThreadedNewsYAML backed by a YAML fixture file
// in a temporary directory. The fixture contains:
//   - "General" (NewsCategory) with one article (ID 1)
//   - "Archive" (NewsBundle) with subcategory "Old News" containing one article (ID 1)
func newTestThreadedNews(t *testing.T) *ThreadedNewsYAML {
	t.Helper()

	tn := hotline.ThreadedNews{
		Categories: map[string]hotline.NewsCategoryListData15{
			"General": {
				Name: "General",
				Type: hotline.NewsCategory,
				Articles: map[uint32]*hotline.NewsArtData{
					1: {
						Title:  "Welcome",
						Poster: "admin",
						Data:   "Hello world",
					},
				},
				SubCats: make(map[string]hotline.NewsCategoryListData15),
			},
			"Archive": {
				Name: "Archive",
				Type: hotline.NewsBundle,
				SubCats: map[string]hotline.NewsCategoryListData15{
					"Old News": {
						Name: "Old News",
						Type: hotline.NewsCategory,
						Articles: map[uint32]*hotline.NewsArtData{
							1: {
								Title:  "Legacy Post",
								Poster: "olduser",
								Data:   "Old content",
							},
						},
						SubCats: make(map[string]hotline.NewsCategoryListData15),
					},
				},
			},
		},
	}

	dir := t.TempDir()
	fp := filepath.Join(dir, "ThreadedNews.yaml")

	data, err := yaml.Marshal(&tn)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(fp, data, 0644))

	result, err := NewThreadedNewsYAML(fp)
	require.NoError(t, err)

	return result
}

func TestThreadedNewsYAML_GetCategories(t *testing.T) {
	tn := newTestThreadedNews(t)

	t.Run("returns top-level categories sorted by name", func(t *testing.T) {
		cats := tn.GetCategories(nil)
		require.Len(t, cats, 2)
		assert.Equal(t, "Archive", cats[0].Name)
		assert.Equal(t, "General", cats[1].Name)
	})

	t.Run("returns subcategories for a bundle", func(t *testing.T) {
		cats := tn.GetCategories([]string{"Archive"})
		require.Len(t, cats, 1)
		assert.Equal(t, "Old News", cats[0].Name)
	})

	t.Run("returns empty slice for path with no children", func(t *testing.T) {
		cats := tn.GetCategories([]string{"Archive", "Old News"})
		assert.Empty(t, cats)
	})
}

func TestThreadedNewsYAML_NewsItem(t *testing.T) {
	tn := newTestThreadedNews(t)

	t.Run("retrieves top-level item", func(t *testing.T) {
		item := tn.NewsItem([]string{"General"})
		assert.Equal(t, "General", item.Name)
		assert.Equal(t, hotline.NewsCategory, item.Type)
	})

	t.Run("retrieves nested item", func(t *testing.T) {
		item := tn.NewsItem([]string{"Archive", "Old News"})
		assert.Equal(t, "Old News", item.Name)
		assert.Equal(t, hotline.NewsCategory, item.Type)
	})
}

func TestThreadedNewsYAML_GetArticle(t *testing.T) {
	tn := newTestThreadedNews(t)

	t.Run("returns existing article", func(t *testing.T) {
		art := tn.GetArticle([]string{"General"}, 1)
		require.NotNil(t, art)
		assert.Equal(t, "Welcome", art.Title)
		assert.Equal(t, "admin", art.Poster)
		assert.Equal(t, "Hello world", art.Data)
	})

	t.Run("returns nil for missing article ID", func(t *testing.T) {
		art := tn.GetArticle([]string{"General"}, 999)
		assert.Nil(t, art)
	})

	t.Run("returns article in nested category", func(t *testing.T) {
		art := tn.GetArticle([]string{"Archive", "Old News"}, 1)
		require.NotNil(t, art)
		assert.Equal(t, "Legacy Post", art.Title)
	})
}

func TestThreadedNewsYAML_ListArticles(t *testing.T) {
	tn := newTestThreadedNews(t)

	t.Run("lists articles for a category with articles", func(t *testing.T) {
		artList, err := tn.ListArticles([]string{"General"})
		require.NoError(t, err)
		assert.Equal(t, 1, artList.Count)
		assert.NotEmpty(t, artList.NewsArtList)
	})

	t.Run("lists articles for nested category", func(t *testing.T) {
		artList, err := tn.ListArticles([]string{"Archive", "Old News"})
		require.NoError(t, err)
		assert.Equal(t, 1, artList.Count)
	})
}

func TestThreadedNewsYAML_PostArticle(t *testing.T) {
	t.Run("posts article to empty category and persists", func(t *testing.T) {
		tn := newTestThreadedNews(t)

		// Create a new empty category first.
		err := tn.CreateGrouping(nil, "Fresh", hotline.NewsCategory)
		require.NoError(t, err)

		article := hotline.NewsArtData{
			Title:  "First Post",
			Poster: "user1",
			Data:   "Content here",
		}
		err = tn.PostArticle([]string{"Fresh"}, 0, article)
		require.NoError(t, err)

		// Verify article was stored with ID 1.
		art := tn.GetArticle([]string{"Fresh"}, 1)
		require.NotNil(t, art)
		assert.Equal(t, "First Post", art.Title)
		assert.Equal(t, "Content here", art.Data)

		// Verify persistence by reloading.
		require.NoError(t, tn.Load())
		art = tn.GetArticle([]string{"Fresh"}, 1)
		require.NotNil(t, art)
		assert.Equal(t, "First Post", art.Title)
	})

	t.Run("posts second article and links to previous", func(t *testing.T) {
		tn := newTestThreadedNews(t)

		// General already has article 1. Post a second one.
		article := hotline.NewsArtData{
			Title:  "Second Post",
			Poster: "user2",
			Data:   "More content",
		}
		err := tn.PostArticle([]string{"General"}, 0, article)
		require.NoError(t, err)

		// New article should be ID 2.
		art := tn.GetArticle([]string{"General"}, 2)
		require.NotNil(t, art)
		assert.Equal(t, "Second Post", art.Title)

		// PrevArt of the new article should point to article 1.
		prevID := binary.BigEndian.Uint32(art.PrevArt[:])
		assert.Equal(t, uint32(1), prevID)

		// NextArt of the first article should point to article 2.
		first := tn.GetArticle([]string{"General"}, 1)
		require.NotNil(t, first)
		nextID := binary.BigEndian.Uint32(first.NextArt[:])
		assert.Equal(t, uint32(2), nextID)
	})

	t.Run("reply sets parent FirstChildArt", func(t *testing.T) {
		tn := newTestThreadedNews(t)

		// Post a reply to article 1 in General.
		reply := hotline.NewsArtData{
			Title:  "Reply",
			Poster: "replier",
			Data:   "I agree",
		}
		err := tn.PostArticle([]string{"General"}, 1, reply)
		require.NoError(t, err)

		// The reply should be article 2.
		replyArt := tn.GetArticle([]string{"General"}, 2)
		require.NotNil(t, replyArt)

		// ParentArt should be set to 1.
		parentID := binary.BigEndian.Uint32(replyArt.ParentArt[:])
		assert.Equal(t, uint32(1), parentID)

		// Parent article's FirstChildArt should now point to 2.
		parent := tn.GetArticle([]string{"General"}, 1)
		require.NotNil(t, parent)
		firstChild := binary.BigEndian.Uint32(parent.FirstChildArt[:])
		assert.Equal(t, uint32(2), firstChild)
	})

	t.Run("returns error for empty news path", func(t *testing.T) {
		tn := newTestThreadedNews(t)
		err := tn.PostArticle(nil, 0, hotline.NewsArtData{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid news path")
	})
}

func TestThreadedNewsYAML_DeleteArticle(t *testing.T) {
	tn := newTestThreadedNews(t)

	// Confirm article exists before deletion.
	art := tn.GetArticle([]string{"General"}, 1)
	require.NotNil(t, art)

	err := tn.DeleteArticle([]string{"General"}, 1, false)
	require.NoError(t, err)

	// Article should be gone.
	art = tn.GetArticle([]string{"General"}, 1)
	assert.Nil(t, art)

	// Verify persistence.
	require.NoError(t, tn.Load())
	art = tn.GetArticle([]string{"General"}, 1)
	assert.Nil(t, art)
}

func TestThreadedNewsYAML_DeleteArticle_EmptyPath(t *testing.T) {
	tn := newTestThreadedNews(t)
	err := tn.DeleteArticle(nil, 1, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid news path")
}

func TestThreadedNewsYAML_CreateGrouping(t *testing.T) {
	t.Run("creates a new top-level bundle", func(t *testing.T) {
		tn := newTestThreadedNews(t)

		err := tn.CreateGrouping(nil, "NewBundle", hotline.NewsBundle)
		require.NoError(t, err)

		cats := tn.GetCategories(nil)
		var names []string
		for _, c := range cats {
			names = append(names, c.Name)
		}
		assert.Contains(t, names, "NewBundle")

		// Verify the type.
		item := tn.NewsItem([]string{"NewBundle"})
		assert.Equal(t, hotline.NewsBundle, item.Type)

		// Verify persistence.
		require.NoError(t, tn.Load())
		item = tn.NewsItem([]string{"NewBundle"})
		assert.Equal(t, "NewBundle", item.Name)
	})

	t.Run("creates a nested category inside a bundle", func(t *testing.T) {
		tn := newTestThreadedNews(t)

		err := tn.CreateGrouping([]string{"Archive"}, "Recent", hotline.NewsCategory)
		require.NoError(t, err)

		cats := tn.GetCategories([]string{"Archive"})
		var names []string
		for _, c := range cats {
			names = append(names, c.Name)
		}
		assert.Contains(t, names, "Recent")
		assert.Contains(t, names, "Old News")

		item := tn.NewsItem([]string{"Archive", "Recent"})
		assert.Equal(t, hotline.NewsCategory, item.Type)
	})
}

func TestThreadedNewsYAML_DeleteNewsItem(t *testing.T) {
	t.Run("deletes a top-level category", func(t *testing.T) {
		tn := newTestThreadedNews(t)

		err := tn.DeleteNewsItem([]string{"General"})
		require.NoError(t, err)

		cats := tn.GetCategories(nil)
		for _, c := range cats {
			assert.NotEqual(t, "General", c.Name)
		}

		// Verify persistence.
		require.NoError(t, tn.Load())
		cats = tn.GetCategories(nil)
		for _, c := range cats {
			assert.NotEqual(t, "General", c.Name)
		}
	})

	t.Run("deletes a nested subcategory", func(t *testing.T) {
		tn := newTestThreadedNews(t)

		err := tn.DeleteNewsItem([]string{"Archive", "Old News"})
		require.NoError(t, err)

		cats := tn.GetCategories([]string{"Archive"})
		assert.Empty(t, cats)
	})
}

func TestThreadedNewsYAML_Load(t *testing.T) {
	t.Run("loads from valid fixture file", func(t *testing.T) {
		tn := newTestThreadedNews(t)

		// Verify data loaded correctly.
		assert.NotNil(t, tn.ThreadedNews.Categories)
		assert.Contains(t, tn.ThreadedNews.Categories, "General")
		assert.Contains(t, tn.ThreadedNews.Categories, "Archive")
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		_, err := NewThreadedNewsYAML("/nonexistent/path/ThreadedNews.yaml")
		assert.Error(t, err)
	})

	t.Run("returns error for invalid YAML", func(t *testing.T) {
		dir := t.TempDir()
		fp := filepath.Join(dir, "bad.yaml")
		require.NoError(t, os.WriteFile(fp, []byte(":::not yaml[[["), 0644))

		_, err := NewThreadedNewsYAML(fp)
		assert.Error(t, err)
	})
}
