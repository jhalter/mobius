package hotline

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strconv"
)

// TrackerRegistration represents the payload a Hotline server sends to a Tracker to register
type TrackerRegistration struct {
	Port        [2]byte // Server's listening TCP port number
	UserCount   int     // Number of users connected to this particular server
	PassID      [4]byte // Random number generated by the server
	Name        string  // Server Name
	Description string  // Description of the server

	readOffset int // Internal offset to track read progress
}

// Read implements io.Reader to write tracker registration payload bytes to slice
func (tr *TrackerRegistration) Read(p []byte) (int, error) {
	userCount := make([]byte, 2)
	binary.BigEndian.PutUint16(userCount, uint16(tr.UserCount))

	buf := slices.Concat(
		[]byte{0x00, 0x01}, // Magic number, always 1
		tr.Port[:],
		userCount,
		[]byte{0x00, 0x00}, // Magic number, always 0
		tr.PassID[:],
		[]byte{uint8(len(tr.Name))},
		[]byte(tr.Name),
		[]byte{uint8(len(tr.Description))},
		[]byte(tr.Description),
	)

	if tr.readOffset >= len(buf) {
		return 0, io.EOF // All bytes have been read
	}

	n := copy(p, buf[tr.readOffset:])
	tr.readOffset += n

	return n, nil
}

// Dialer interface to abstract the dialing operation
type Dialer interface {
	Dial(network, address string) (net.Conn, error)
}

// RealDialer is the real implementation of the Dialer interface
type RealDialer struct{}

func (d *RealDialer) Dial(network, address string) (net.Conn, error) {
	return net.Dial(network, address)
}

func register(dialer Dialer, tracker string, tr io.Reader) error {
	conn, err := dialer.Dial("udp", tracker)
	if err != nil {
		return fmt.Errorf("failed to dial tracker: %w", err)
	}
	defer conn.Close()

	if _, err := io.Copy(conn, tr); err != nil {
		return fmt.Errorf("failed to write to connection: %w", err)
	}

	return nil
}

// All string values use 8-bit ASCII character set encoding.
// Client Interface with Tracker
// After establishing a connection with tracker, the following information is sent:
// Description	Size	Data	Note
// Magic number	4	‘HTRK’
// Version	2	1 or 2	Old protocol (1) or new (2)

// TrackerHeader is sent in reply Reply received from the tracker starts with a header:
type TrackerHeader struct {
	Protocol [4]byte // "HTRK" 0x4854524B
	Version  [2]byte // Old protocol (1) or new (2)
}

type ServerInfoHeader struct {
	MsgType     [2]byte // Always has value of 1
	MsgDataSize [2]byte // Remaining size of request
	SrvCount    [2]byte // Number of servers in the server list
	SrvCountDup [2]byte // Same as previous field ¯\_(ツ)_/¯
}

// ServerRecord is a tracker listing for a single server
type ServerRecord struct {
	IPAddr          [4]byte
	Port            [2]byte
	NumUsers        [2]byte // Number of users connected to this particular server
	Unused          [2]byte
	NameSize        byte   // Length of Name string
	Name            []byte // Server Name
	DescriptionSize byte
	Description     []byte
}

func GetListing(conn io.ReadWriteCloser) ([]ServerRecord, error) {
	defer func() { _ = conn.Close() }()

	_, err := conn.Write(
		[]byte{
			0x48, 0x54, 0x52, 0x4B, // HTRK
			0x00, 0x01, // Version
		},
	)
	if err != nil {
		return nil, err
	}

	var th TrackerHeader
	if err := binary.Read(conn, binary.BigEndian, &th); err != nil {
		return nil, err
	}

	var info ServerInfoHeader
	if err := binary.Read(conn, binary.BigEndian, &info); err != nil {
		return nil, err
	}

	totalSrv := int(binary.BigEndian.Uint16(info.SrvCount[:]))

	scanner := bufio.NewScanner(conn)
	scanner.Split(serverScanner)

	var servers []ServerRecord
	for {
		scanner.Scan()

		var srv ServerRecord
		_, err = srv.Write(scanner.Bytes())
		if err != nil {
			return nil, err
		}

		servers = append(servers, srv)
		if len(servers) == totalSrv {
			break
		}
	}

	return servers, nil
}

// serverScanner implements bufio.SplitFunc for parsing the tracker list into ServerRecords tokens
// Example payload:
// 00000000  18 05 30 63 15 7c 00 02  00 00 10 54 68 65 20 4d  |..0c.|.....The M|
// 00000010  6f 62 69 75 73 20 53 74  72 69 70 40 48 6f 6d 65  |obius Strip@Home|
// 00000020  20 6f 66 20 74 68 65 20  4d 6f 62 69 75 73 20 48  | of the Mobius H|
// 00000030  6f 74 6c 69 6e 65 20 73  65 72 76 65 72 20 61 6e  |otline server an|
// 00000040  64 20 63 6c 69 65 6e 74  20 7c 20 54 52 54 50 48  |d client | TRTPH|
// 00000050  4f 54 4c 2e 63 6f 6d 3a  35 35 30 30 2d 4f 3a b2  |OTL.com:5500-O:.|
// 00000060  15 7c 00 00 00 00 08 53  65 6e 65 63 74 75 73 20  |.|.....Senectus |
func serverScanner(data []byte, _ bool) (advance int, token []byte, err error) {
	// The name length field is the 11th byte of the server record.  If we don't have that many bytes,
	// return nil token so the Scanner reads more data and continues scanning.
	if len(data) < 10 {
		return 0, nil, nil
	}

	// A server entry has two variable length fields: the name and description.
	// To get the token length, we first need the name length from the 10th byte
	nameLen := int(data[10])

	// The description length field is at the 12th + nameLen byte of the server record.
	// If we don't have that many bytes, return nil token so the Scanner reads more data and continues scanning.
	if len(data) < 11+nameLen {
		return 0, nil, nil
	}

	// Next we need the description length from the 11+nameLen byte:
	descLen := int(data[11+nameLen])

	if len(data) < 12+nameLen+descLen {
		return 0, nil, nil
	}

	return 12 + nameLen + descLen, data[0 : 12+nameLen+descLen], nil
}

// Write implements io.Writer for ServerRecord
func (s *ServerRecord) Write(b []byte) (n int, err error) {
	if len(b) < 13 {
		return 0, errors.New("too few bytes")
	}
	copy(s.IPAddr[:], b[0:4])
	copy(s.Port[:], b[4:6])
	copy(s.NumUsers[:], b[6:8])
	s.NameSize = b[10]
	nameLen := int(b[10])

	s.Name = b[11 : 11+nameLen]
	s.DescriptionSize = b[11+nameLen]
	s.Description = b[12+nameLen : 12+nameLen+int(s.DescriptionSize)]

	return 12 + nameLen + int(s.DescriptionSize), nil
}

func (s *ServerRecord) Addr() string {
	return fmt.Sprintf("%s:%s",
		net.IP(s.IPAddr[:]),
		strconv.Itoa(int(binary.BigEndian.Uint16(s.Port[:]))),
	)
}
