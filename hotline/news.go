package hotline

import (
	"cmp"
	"encoding/binary"
	"io"
	"slices"
)

var (
	NewsBundle   = [2]byte{0, 2}
	NewsCategory = [2]byte{0, 3}
)

type ThreadedNewsMgr interface {
	ListArticles(newsPath []string) NewsArtListData
	GetArticle(newsPath []string, articleID uint32) *NewsArtData
	DeleteArticle(newsPath []string, articleID uint32, recursive bool) error
	PostArticle(newsPath []string, parentArticleID uint32, article NewsArtData) error
	CreateGrouping(newsPath []string, name string, t [2]byte) error
	GetCategories(paths []string) []NewsCategoryListData15
	NewsItem(newsPath []string) NewsCategoryListData15
	DeleteNewsItem(newsPath []string) error
}

// ThreadedNews contains the top level of threaded news categories, bundles, and articles.
type ThreadedNews struct {
	Categories map[string]NewsCategoryListData15 `yaml:"Categories"`
}

type NewsCategoryListData15 struct {
	Type     [2]byte                           `yaml:"Type,flow"` // Bundle (2) or category (3)
	Name     string                            `yaml:"Name"`
	Articles map[uint32]*NewsArtData           `yaml:"Articles"` // Optional, if Type is Category
	SubCats  map[string]NewsCategoryListData15 `yaml:"SubCats"`
	GUID     [16]byte                          `yaml:"-"` // What does this do?  Undocumented and seeming unused.
	AddSN    [4]byte                           `yaml:"-"` // What does this do?  Undocumented and seeming unused.
	DeleteSN [4]byte                           `yaml:"-"` // What does this do?  Undocumented and seeming unused.

	readOffset int // Internal offset to track read progress
}

func (newscat *NewsCategoryListData15) GetNewsArtListData() NewsArtListData {
	var newsArts []NewsArtList
	var newsArtsPayload []byte

	for i, art := range newscat.Articles {
		id := make([]byte, 4)
		binary.BigEndian.PutUint32(id, i)

		newsArts = append(newsArts, NewsArtList{
			ID:          [4]byte(id),
			TimeStamp:   art.Date,
			ParentID:    art.ParentArt,
			Title:       []byte(art.Title),
			Poster:      []byte(art.Poster),
			ArticleSize: art.DataSize(),
		})
	}

	// Sort the articles by ID.  This is important for displaying the message threading correctly on the client side.
	slices.SortFunc(newsArts, func(a, b NewsArtList) int {
		return cmp.Compare(
			binary.BigEndian.Uint32(a.ID[:]),
			binary.BigEndian.Uint32(b.ID[:]),
		)
	})

	for _, v := range newsArts {
		b, err := io.ReadAll(&v)
		if err != nil {
			// TODO
			panic(err)
		}
		newsArtsPayload = append(newsArtsPayload, b...)
	}

	return NewsArtListData{
		Count:       len(newsArts),
		Name:        []byte{},
		Description: []byte{},
		NewsArtList: newsArtsPayload,
	}
}

// NewsArtData represents an individual news article.
type NewsArtData struct {
	Title         string  `yaml:"Title"`
	Poster        string  `yaml:"Poster"`
	Date          [8]byte `yaml:"Date,flow"`
	PrevArt       [4]byte `yaml:"PrevArt,flow"`
	NextArt       [4]byte `yaml:"NextArt,flow"`
	ParentArt     [4]byte `yaml:"ParentArt,flow"`
	FirstChildArt [4]byte `yaml:"FirstChildArtArt,flow"`
	DataFlav      []byte  `yaml:"-"` // MIME type string.  Always "text/plain".
	Data          string  `yaml:"Data"`
}

func (art *NewsArtData) DataSize() [2]byte {
	dataLen := make([]byte, 2)
	binary.BigEndian.PutUint16(dataLen, uint16(len(art.Data)))

	return [2]byte(dataLen)
}

type NewsArtListData struct {
	ID          [4]byte `yaml:"Type"`
	Name        []byte  `yaml:"Name"`
	Description []byte  `yaml:"Description"` // not used?
	NewsArtList []byte  // List of articles			Optional (if article count > 0)
	Count       int

	readOffset int // Internal offset to track read progress
}

func (nald *NewsArtListData) Read(p []byte) (int, error) {
	count := make([]byte, 4)
	binary.BigEndian.PutUint32(count, uint32(nald.Count))

	buf := slices.Concat(
		nald.ID[:],
		count,
		[]byte{uint8(len(nald.Name))},
		nald.Name,
		[]byte{uint8(len(nald.Description))},
		nald.Description,
		nald.NewsArtList,
	)

	if nald.readOffset >= len(buf) {
		return 0, io.EOF // All bytes have been read
	}
	n := copy(p, buf[nald.readOffset:])
	nald.readOffset += n

	return n, nil
}

// NewsArtList is a summarized version of a NewArtData record for display in list view
type NewsArtList struct {
	ID          [4]byte
	TimeStamp   [8]byte // Year (2 bytes), milliseconds (2 bytes) and seconds (4 bytes)
	ParentID    [4]byte
	Flags       [4]byte
	FlavorCount [2]byte
	// Title size	1
	Title []byte // string
	// Poster size	1
	// Poster	Poster string
	Poster     []byte
	FlavorList []NewsFlavorList
	// Flavor list…			Optional (if flavor count > 0)
	ArticleSize [2]byte // Size 2

	readOffset int // Internal offset to track read progress
}

var (
	NewsFlavorLen = []byte{0x0a}
	NewsFlavor    = []byte("text/plain")
)

func (nal *NewsArtList) Read(p []byte) (int, error) {
	out := slices.Concat(
		nal.ID[:],
		nal.TimeStamp[:],
		nal.ParentID[:],
		nal.Flags[:],
		[]byte{0, 1}, // Flavor Count TODO: make this not hardcoded
		[]byte{uint8(len(nal.Title))},
		nal.Title,
		[]byte{uint8(len(nal.Poster))},
		nal.Poster,
		NewsFlavorLen,
		NewsFlavor,
		nal.ArticleSize[:],
	)

	if nal.readOffset >= len(out) {
		return 0, io.EOF // All bytes have been read
	}

	n := copy(p, out[nal.readOffset:])
	nal.readOffset += n

	return n, io.EOF
}

type NewsFlavorList struct {
	// Flavor size	1
	// Flavor text	size		MIME type string
	// Article size	2
}

func (newscat *NewsCategoryListData15) Read(p []byte) (int, error) {
	count := make([]byte, 2)
	binary.BigEndian.PutUint16(count, uint16(len(newscat.Articles)+len(newscat.SubCats)))

	out := slices.Concat(
		newscat.Type[:],
		count,
	)
	if newscat.Type == NewsCategory {
		out = slices.Concat(out,
			newscat.GUID[:],
			newscat.AddSN[:],
			newscat.DeleteSN[:],
		)
	}
	out = slices.Concat(out,
		newscat.nameLen(),
		[]byte(newscat.Name),
	)

	if newscat.readOffset >= len(out) {
		return 0, io.EOF // All bytes have been read
	}

	n := copy(p, out)

	newscat.readOffset = n

	return n, nil
}

func (newscat *NewsCategoryListData15) nameLen() []byte {
	return []byte{uint8(len(newscat.Name))}
}

// newsPathScanner implements bufio.SplitFunc for parsing incoming byte slices into complete tokens
func newsPathScanner(data []byte, _ bool) (advance int, token []byte, err error) {
	if len(data) < 3 {
		return 0, nil, nil
	}

	advance = 3 + int(data[2])
	return advance, data[3:advance], nil
}
