package hotline

import (
	"bytes"
	"encoding/binary"
	"io"
	"slices"
	"sort"
)

const defaultNewsDateFormat = "Jan02 15:04" // Jun23 20:49

const defaultNewsTemplate = `From %s (%s):

%s

__________________________________________________________`

// ThreadedNews is the top level struct containing all threaded news categories, bundles, and articles
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
}

func (newscat *NewsCategoryListData15) GetNewsArtListData() NewsArtListData {
	var newsArts []NewsArtList
	var newsArtsPayload []byte

	for i, art := range newscat.Articles {
		id := make([]byte, 4)
		binary.BigEndian.PutUint32(id, i)

		newArt := NewsArtList{
			ID:          [4]byte(id),
			TimeStamp:   art.Date,
			ParentID:    art.ParentArt,
			Title:       []byte(art.Title),
			Poster:      []byte(art.Poster),
			ArticleSize: art.DataSize(),
		}

		newsArts = append(newsArts, newArt)
	}

	sort.Sort(byID(newsArts))

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

// NewsArtData represents single news article
type NewsArtData struct {
	Title         string  `yaml:"Title"`
	Poster        string  `yaml:"Poster"`
	Date          [8]byte `yaml:"Date,flow"`
	PrevArt       [4]byte `yaml:"PrevArt,flow"`
	NextArt       [4]byte `yaml:"NextArt,flow"`
	ParentArt     [4]byte `yaml:"ParentArt,flow"`
	FirstChildArt [4]byte `yaml:"FirstChildArtArt,flow"`
	DataFlav      []byte  `yaml:"-"` // "text/plain"
	Data          string  `yaml:"Data"`
}

func (art *NewsArtData) DataSize() []byte {
	dataLen := make([]byte, 2)
	binary.BigEndian.PutUint16(dataLen, uint16(len(art.Data)))

	return dataLen
}

type NewsArtListData struct {
	ID          [4]byte `yaml:"ID"`
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
	ArticleSize []byte // Size 2

	readOffset int // Internal offset to track read progress
}

type byID []NewsArtList

func (s byID) Len() int {
	return len(s)
}
func (s byID) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s byID) Less(i, j int) bool {
	return binary.BigEndian.Uint32(s[i].ID[:]) < binary.BigEndian.Uint32(s[j].ID[:])
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
		[]byte{0, 1}, // Flavor Count
		[]byte{uint8(len(nal.Title))},
		nal.Title,
		[]byte{uint8(len(nal.Poster))},
		nal.Poster,
		NewsFlavorLen,
		NewsFlavor,
		nal.ArticleSize,
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

func (newscat *NewsCategoryListData15) MarshalBinary() (data []byte, err error) {
	count := make([]byte, 2)
	binary.BigEndian.PutUint16(count, uint16(len(newscat.Articles)+len(newscat.SubCats)))

	out := append(newscat.Type[:], count...)

	// If type is category
	if bytes.Equal(newscat.Type[:], []byte{0, 3}) {
		out = append(out, newscat.GUID[:]...)     // GUID
		out = append(out, newscat.AddSN[:]...)    // Add SN
		out = append(out, newscat.DeleteSN[:]...) // Delete SN
	}

	out = append(out, newscat.nameLen()...)
	out = append(out, []byte(newscat.Name)...)

	return out, err
}

func (newscat *NewsCategoryListData15) nameLen() []byte {
	return []byte{uint8(len(newscat.Name))}
}

// TODO: re-implement as bufio.Scanner interface
func ReadNewsPath(newsPath []byte) []string {
	if len(newsPath) == 0 {
		return []string{}
	}
	pathCount := binary.BigEndian.Uint16(newsPath[0:2])

	pathData := newsPath[2:]
	var paths []string

	for i := uint16(0); i < pathCount; i++ {
		pathLen := pathData[2]
		paths = append(paths, string(pathData[3:3+pathLen]))

		pathData = pathData[pathLen+3:]
	}

	return paths
}

func (s *Server) GetNewsCatByPath(paths []string) map[string]NewsCategoryListData15 {
	cats := s.ThreadedNews.Categories
	for _, path := range paths {
		cats = cats[path].SubCats
	}
	return cats
}
