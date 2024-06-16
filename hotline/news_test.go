package hotline

import (
	"reflect"
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
				0x00, 0x00, 0x00, 0x01,
				0x00, 0x00, 0x00, 0x02,
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
			gotData, err := newscat.MarshalBinary()
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
			if !reflect.DeepEqual(gotData, tt.wantData) {
				t.Errorf("MarshalBinary() gotData = %v, want %v", gotData, tt.wantData)
			}
		})
	}
}
