package mobius

import (
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type BanFile struct {
	banList  map[string]*time.Time
	filePath string

	sync.Mutex
}

func NewBanFile(path string) (*BanFile, error) {
	bf := &BanFile{
		filePath: path,
		banList:  make(map[string]*time.Time),
	}

	err := bf.Load()

	return bf, err
}

func (bf *BanFile) Load() error {
	bf.Lock()
	defer bf.Unlock()

	bf.banList = make(map[string]*time.Time)

	fh, err := os.Open(bf.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer fh.Close()

	decoder := yaml.NewDecoder(fh)
	err = decoder.Decode(&bf.banList)
	if err != nil {
		return err
	}

	return nil
}

func (bf *BanFile) Add(ip string, until *time.Time) error {
	bf.Lock()
	defer bf.Unlock()

	bf.banList[ip] = until

	out, err := yaml.Marshal(bf.banList)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(bf.filePath), out, 0644)
}

func (bf *BanFile) IsBanned(ip string) (bool, *time.Time) {
	bf.Lock()
	defer bf.Unlock()

	if until, ok := bf.banList[ip]; ok {
		return true, until
	}

	return false, nil
}
