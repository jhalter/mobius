package hotline

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"io"
	"slices"
	"sort"
)

const defaultNewsDateFormat = "Jan02 15:04" // Jun23 20:49

const defaultNewsTemplate = `From %s (%s):

%s

__________________________________________________________`

type ThreadedNews struct {
	Categories map[string]NewsCategoryListData15 `yaml:"Categories"`
}

type NewsCategoryListData15 struct {
	Type     [2]byte `yaml:"Type"` // Size 2 ; Bundle (2) or category (3)
	Count    []byte  // Article or SubCategory count Size 2
	NameSize byte
	Name     string                            `yaml:"Name"`     //
	Articles map[uint32]*NewsArtData           `yaml:"Articles"` // Optional, if Type is Category
	SubCats  map[string]NewsCategoryListData15 `yaml:"SubCats"`
	GUID     []byte                            // Size 16
	AddSN    []byte                            // Size 4
	DeleteSN []byte                            // Size 4
}

func (newscat *NewsCategoryListData15) GetNewsArtListData() NewsArtListData {
	var newsArts []NewsArtList
	var newsArtsPayload []byte

	for i, art := range newscat.Articles {
		id := make([]byte, 4)
		binary.BigEndian.PutUint32(id, i)

		newArt := NewsArtList{
			ID:          id,
			TimeStamp:   art.Date,
			ParentID:    art.ParentArt,
			Flags:       []byte{0, 0, 0, 0},
			FlavorCount: []byte{0, 0},
			Title:       []byte(art.Title),
			Poster:      []byte(art.Poster),
			ArticleSize: art.DataSize(),
		}
		newsArts = append(newsArts, newArt)
	}

	sort.Sort(byID(newsArts))

	for _, v := range newsArts {
		newsArtsPayload = append(newsArtsPayload, v.Payload()...)
	}

	nald := NewsArtListData{
		ID:          [4]byte{0, 0, 0, 0},
		Count:       len(newsArts),
		Name:        []byte{},
		Description: []byte{},
		NewsArtList: newsArtsPayload,
	}

	return nald
}

// NewsArtData represents single news article
type NewsArtData struct {
	Title         string `yaml:"Title"`
	Poster        string `yaml:"Poster"`
	Date          []byte `yaml:"Date"`             // size 8
	PrevArt       []byte `yaml:"PrevArt"`          // size 4
	NextArt       []byte `yaml:"NextArt"`          // size 4
	ParentArt     []byte `yaml:"ParentArt"`        // size 4
	FirstChildArt []byte `yaml:"FirstChildArtArt"` // size 4
	DataFlav      []byte `yaml:"DataFlav"`         // "text/plain"
	Data          string `yaml:"Data"`
}

func (art *NewsArtData) DataSize() []byte {
	dataLen := make([]byte, 2)
	binary.BigEndian.PutUint16(dataLen, uint16(len(art.Data)))

	return dataLen
}

type NewsArtListData struct {
	ID          [4]byte `yaml:"ID"` // Size 4
	Name        []byte  `yaml:"Name"`
	Description []byte  `yaml:"Description"` // not used?
	NewsArtList []byte  // List of articles			Optional (if article count > 0)
	Count       int
}

func (nald *NewsArtListData) Read(p []byte) (int, error) {
	count := make([]byte, 4)
	binary.BigEndian.PutUint32(count, uint32(nald.Count))

	return copy(
			p,
			slices.Concat(
				nald.ID[:],
				count,
				[]byte{uint8(len(nald.Name))},
				nald.Name,
				[]byte{uint8(len(nald.Description))},
				nald.Description,
				nald.NewsArtList,
			),
		),
		io.EOF
}

// NewsArtList is a summarized version of a NewArtData record for display in list view
type NewsArtList struct {
	ID          []byte // Size 4
	TimeStamp   []byte // Year (2 bytes), milliseconds (2 bytes) and seconds (4 bytes)
	ParentID    []byte // Size 4
	Flags       []byte // Size 4
	FlavorCount []byte // Size 2
	// Title size	1
	Title []byte // string
	// Poster size	1
	// Poster	Poster string
	Poster     []byte
	FlavorList []NewsFlavorList
	// Flavor list…			Optional (if flavor count > 0)
	ArticleSize []byte // Size 2
}

type byID []NewsArtList

func (s byID) Len() int {
	return len(s)
}
func (s byID) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s byID) Less(i, j int) bool {
	return binary.BigEndian.Uint32(s[i].ID) < binary.BigEndian.Uint32(s[j].ID)
}

func (nal *NewsArtList) Payload() []byte {
	out := append(nal.ID, nal.TimeStamp...)
	out = append(out, nal.ParentID...)
	out = append(out, nal.Flags...)

	out = append(out, []byte{0, 1}...)

	out = append(out, []byte{uint8(len(nal.Title))}...)
	out = append(out, nal.Title...)
	out = append(out, []byte{uint8(len(nal.Poster))}...)
	out = append(out, nal.Poster...)
	out = append(out, []byte{0x0a, 0x74, 0x65, 0x78, 0x74, 0x2f, 0x70, 0x6c, 0x61, 0x69, 0x6e}...) // TODO: wat?
	out = append(out, nal.ArticleSize...)

	return out
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

	if bytes.Equal(newscat.Type[:], []byte{0, 3}) {
		// Generate a random GUID // TODO: does this need to be random?
		b := make([]byte, 16)
		_, err := rand.Read(b)
		if err != nil {
			return data, err
		}

		out = append(out, b...)                  // GUID
		out = append(out, []byte{0, 0, 0, 1}...) // Add SN (TODO: not sure what this is)
		out = append(out, []byte{0, 0, 0, 2}...) // Delete SN (TODO: not sure what this is)
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
