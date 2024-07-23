package mobius

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"github.com/jhalter/mobius/hotline"
	"gopkg.in/yaml.v3"
	"os"
	"slices"
	"sort"
	"sync"
)

type ThreadedNewsYAML struct {
	ThreadedNews hotline.ThreadedNews

	filePath string

	mu sync.Mutex
}

func NewThreadedNewsYAML(filePath string) (*ThreadedNewsYAML, error) {
	tn := &ThreadedNewsYAML{filePath: filePath}

	err := tn.Load()

	return tn, err
}

func (n *ThreadedNewsYAML) CreateGrouping(newsPath []string, name string, t [2]byte) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	cats := n.getCatByPath(newsPath)
	cats[name] = hotline.NewsCategoryListData15{
		Name:     name,
		Type:     t,
		Articles: map[uint32]*hotline.NewsArtData{},
		SubCats:  make(map[string]hotline.NewsCategoryListData15),
	}

	return n.writeFile()
}

func (n *ThreadedNewsYAML) NewsItem(newsPath []string) hotline.NewsCategoryListData15 {
	n.mu.Lock()
	defer n.mu.Unlock()

	cats := n.ThreadedNews.Categories
	delName := newsPath[len(newsPath)-1]
	if len(newsPath) > 1 {
		for _, fp := range newsPath[0 : len(newsPath)-1] {
			cats = cats[fp].SubCats
		}
	}

	return cats[delName]
}

func (n *ThreadedNewsYAML) DeleteNewsItem(newsPath []string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	cats := n.ThreadedNews.Categories
	delName := newsPath[len(newsPath)-1]
	if len(newsPath) > 1 {
		for _, fp := range newsPath[0 : len(newsPath)-1] {
			cats = cats[fp].SubCats
		}
	}

	delete(cats, delName)

	return n.writeFile()
}

func (n *ThreadedNewsYAML) GetArticle(newsPath []string, articleID uint32) *hotline.NewsArtData {
	n.mu.Lock()
	defer n.mu.Unlock()

	var cat hotline.NewsCategoryListData15
	cats := n.ThreadedNews.Categories

	for _, fp := range newsPath {
		cat = cats[fp]
		cats = cats[fp].SubCats
	}

	art := cat.Articles[articleID]
	if art == nil {
		return nil
	}

	return art
}

func (n *ThreadedNewsYAML) GetCategories(paths []string) []hotline.NewsCategoryListData15 {
	n.mu.Lock()
	defer n.mu.Unlock()

	var categories []hotline.NewsCategoryListData15
	for _, c := range n.getCatByPath(paths) {
		categories = append(categories, c)
	}

	slices.SortFunc(categories, func(a, b hotline.NewsCategoryListData15) int {
		return cmp.Compare(
			a.Name,
			b.Name,
		)
	})

	return categories
}

func (n *ThreadedNewsYAML) getCatByPath(paths []string) map[string]hotline.NewsCategoryListData15 {
	cats := n.ThreadedNews.Categories
	for _, path := range paths {
		cats = cats[path].SubCats
	}

	return cats
}

func (n *ThreadedNewsYAML) PostArticle(newsPath []string, parentArticleID uint32, article hotline.NewsArtData) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	binary.BigEndian.PutUint32(article.ParentArt[:], parentArticleID)

	if len(newsPath) == 0 {
		return fmt.Errorf("invalid news path")
	}

	cats := n.getCatByPath(newsPath[:len(newsPath)-1])

	catName := newsPath[len(newsPath)-1]
	cat := cats[catName]

	var keys []int
	for k := range cat.Articles {
		keys = append(keys, int(k))
	}

	nextID := uint32(1)
	if len(keys) > 0 {
		sort.Ints(keys)
		prevID := uint32(keys[len(keys)-1])
		nextID = prevID + 1

		binary.BigEndian.PutUint32(article.PrevArt[:], prevID)

		// Set next article Type
		binary.BigEndian.PutUint32(cat.Articles[prevID].NextArt[:], nextID)
	}

	// Update parent article with first child reply
	parentID := parentArticleID
	if parentID != 0 {
		parentArt := cat.Articles[parentID]

		if parentArt.FirstChildArt == [4]byte{0, 0, 0, 0} {
			binary.BigEndian.PutUint32(parentArt.FirstChildArt[:], nextID)
		}
	}

	cat.Articles[nextID] = &article

	cats[catName] = cat

	return n.writeFile()
}

func (n *ThreadedNewsYAML) DeleteArticle(newsPath []string, articleID uint32, _ bool) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	//if recursive {
	//	// TODO: Handle delete recursive
	//}

	if len(newsPath) == 0 {
		return fmt.Errorf("invalid news path")
	}

	cats := n.getCatByPath(newsPath[:len(newsPath)-1])

	catName := newsPath[len(newsPath)-1]

	cat := cats[catName]
	delete(cat.Articles, articleID)
	cats[catName] = cat

	return n.writeFile()
}

func (n *ThreadedNewsYAML) ListArticles(newsPath []string) hotline.NewsArtListData {
	n.mu.Lock()
	defer n.mu.Unlock()

	var cat hotline.NewsCategoryListData15
	cats := n.ThreadedNews.Categories

	for _, fp := range newsPath {
		cat = cats[fp]
		cats = cats[fp].SubCats
	}

	return cat.GetNewsArtListData()
}

func (n *ThreadedNewsYAML) Load() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	fh, err := os.Open(n.filePath)
	if err != nil {
		return err
	}
	defer fh.Close()

	n.ThreadedNews = hotline.ThreadedNews{}

	return yaml.NewDecoder(fh).Decode(&n.ThreadedNews)
}

func (n *ThreadedNewsYAML) writeFile() error {
	out, err := yaml.Marshal(&n.ThreadedNews)
	if err != nil {
		return err
	}

	// Define a temporary file path in the same directory.
	tempFilePath := n.filePath + ".tmp"

	// Write the marshaled YAML to the temporary file.
	if err := os.WriteFile(tempFilePath, out, 0644); err != nil {
		return fmt.Errorf("write to temporary file: %v", err)
	}

	// Atomically rename the temporary file to the final file path.
	if err := os.Rename(tempFilePath, n.filePath); err != nil {
		return fmt.Errorf("rename temporary file to final file: %v", err)
	}

	return nil
}
