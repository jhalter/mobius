package mobius

import (
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
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
	if err != nil {
		return nil, fmt.Errorf("load ban file: %w", err)
	}

	return bf, nil
}

func (bf *BanFile) Load() error {
	bf.Lock()
	defer bf.Unlock()

	bf.banList = make(map[string]*time.Time)

	fh, err := os.Open(bf.filePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open file: %v", err)
	}
	defer func() { _ = fh.Close() }()

	err = yaml.NewDecoder(fh).Decode(&bf.banList)
	if err != nil {
		return fmt.Errorf("decode yaml: %v", err)
	}

	return nil
}

func (bf *BanFile) Add(ip string, until *time.Time) error {
	bf.Lock()
	defer bf.Unlock()

	bf.banList[ip] = until

	out, err := yaml.Marshal(bf.banList)
	if err != nil {
		return fmt.Errorf("marshal yaml: %v", err)
	}

	err = os.WriteFile(path.Join(bf.filePath), out, 0644)
	if err != nil {
		return fmt.Errorf("write file: %v", err)
	}

	return nil
}

func (bf *BanFile) IsBanned(ip string) (bool, *time.Time) {
	bf.Lock()
	defer bf.Unlock()

	if until, ok := bf.banList[ip]; ok {
		return true, until
	}

	return false, nil
}
