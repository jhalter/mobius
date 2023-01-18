package hotline

import (
	"encoding/binary"
	"errors"
)

func byteToInt(bytes []byte) (int, error) {
	switch len(bytes) {
	case 2:
		return int(binary.BigEndian.Uint16(bytes)), nil
	case 4:
		return int(binary.BigEndian.Uint32(bytes)), nil
	}

	return 0, errors.New("unknown byte length")
}
