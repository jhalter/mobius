package hotline

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/jhalter/mobius/concat"
	"net"
	"strconv"
	"time"
)

type TrackerRegistration struct {
	Port        []byte // Server listening port number
	UserCount   int    // Number of users connected to this particular server
	PassID      []byte // Random number generated by the server
	Name        string // Server name
	Description string // Description of the server
}

func (tr *TrackerRegistration) Payload() []byte {
	userCount := make([]byte, 2)
	binary.BigEndian.PutUint16(userCount, uint16(tr.UserCount))

	return concat.Slices(
		[]byte{0x00, 0x01},
		tr.Port,
		userCount,
		[]byte{0x00, 0x00},
		tr.PassID,
		[]byte{uint8(len(tr.Name))},
		[]byte(tr.Name),
		[]byte{uint8(len(tr.Description))},
		[]byte(tr.Description),
	)
}

func register(tracker string, tr TrackerRegistration) error {
	conn, err := net.Dial("udp", tracker)
	if err != nil {
		return err
	}

	if _, err := conn.Write(tr.Payload()); err != nil {
		return err
	}

	return nil
}

type ServerListing struct {
}

const trackerTimeout = 5 * time.Second

// All string values use 8-bit ASCII character set encoding.
// Client Interface with Tracker
// After establishing a connection with tracker, the following information is sent:
// Description	Size	Data	Note
// Magic number	4	‘HTRK’
// Version	2	1 or 2	Old protocol (1) or new (2)

// Reply received from the tracker starts with a header:
type TrackerHeader struct {
	Protocol [4]byte // "HTRK" 0x4854524B
	Version  [2]byte // Old protocol (1) or new (2)
}

//Message type	2	1	Sending list of servers
//Message data size	2		Remaining size of this request
//Number of servers	2		Number of servers in the server list
//Number of servers	2		Same as previous field
type ServerInfoHeader struct {
	MsgType     [2]byte // always has value of 1
	MsgDataSize [2]byte // Remaining size of request
	SrvCount    [2]byte // Number of servers in the server list
	SrvCountDup [2]byte // Same as previous field ¯\_(ツ)_/¯
}

type ServerRecord struct {
	IPAddr          []byte
	Port            []byte
	NumUsers        []byte // Number of users connected to this particular server
	Unused          []byte
	NameSize        byte   // Length of name string
	Name            []byte // Server’s name
	DescriptionSize byte
	Description     []byte
}

func GetListing(addr string) ([]ServerRecord, error) {
	conn, err := net.DialTimeout("tcp", addr, trackerTimeout)
	defer func() { _ = conn.Close() }()

	_, err = conn.Write(
		[]byte{
			0x48, 0x54, 0x52, 0x4B, // HTRK
			0x00, 0x01, // Version
		},
	)
	if err != nil {
		return nil, err
	}

	totalRead := 0

	buf := make([]byte, 4096)
	var readLen int
	if readLen, err = conn.Read(buf); err != nil {
		return nil, err
	}
	totalRead += readLen // 1514

	var th TrackerHeader
	if err := binary.Read(bytes.NewReader(buf[:6]), binary.BigEndian, &th); err != nil {
		return nil, err
	}

	var info ServerInfoHeader
	if err := binary.Read(bytes.NewReader(buf[6:14]), binary.BigEndian, &info); err != nil {
		return nil, err
	}

	payloadSize := int(binary.BigEndian.Uint16(info.MsgDataSize[:]))

	buf = buf[:readLen]
	if totalRead < payloadSize {
		for {
			tmpBuf := make([]byte, 4096)
			if readLen, err = conn.Read(tmpBuf); err != nil {
				return nil, err
			}
			buf = append(buf, tmpBuf[:readLen]...)
			totalRead += readLen
			if totalRead >= payloadSize {
				break
			}
		}
	}
	totalSrv := int(binary.BigEndian.Uint16(info.SrvCount[:]))

	srvBuf := buf[14:totalRead]
	totalRead += readLen

	var servers []ServerRecord

	for {
		var srv ServerRecord
		n, _ := srv.Read(srvBuf)
		servers = append(servers, srv)

		srvBuf = srvBuf[n:]

		if len(servers) == totalSrv {
			return servers, nil
		}

		if len(srvBuf) == 0 {
			return servers, errors.New("tracker sent too few bytes for server count")
		}
	}
}

func (s *ServerRecord) Read(b []byte) (n int, err error) {
	s.IPAddr = b[0:4]
	s.Port = b[4:6]
	s.NumUsers = b[6:8]
	s.NameSize = b[10]
	nameLen := int(b[10])
	s.Name = b[11 : 11+nameLen]
	s.DescriptionSize = b[11+nameLen]
	s.Description = b[12+nameLen : 12+nameLen+int(s.DescriptionSize)]

	return 12 + nameLen + int(s.DescriptionSize), nil
}

func (s *ServerRecord) PortInt() int {
	data := binary.BigEndian.Uint16(s.Port)
	return int(data)
}

func (s *ServerRecord) Addr() string {
	return fmt.Sprintf("%s:%s",
		net.IP(s.IPAddr),
		strconv.Itoa(int(binary.BigEndian.Uint16(s.Port))),
	)
}
