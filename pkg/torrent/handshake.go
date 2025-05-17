package torrent

import (
	"fmt"
	"net"
)

type Handshake struct {
	PeerID   []byte
	InfoHash [20]byte
}

func (h *Handshake) ToBytes() []byte {
	var b []byte
	b = append(b, 19)
	b = append(b, "BitTorrent protocol"...)

	for i := 0; i < 8; i++ {
		b = append(b, 0)
	}

	b = append(b, h.InfoHash[:]...)
	b = append(b, h.PeerID...)

	return b
}

func HandshakeFromBytes(b []byte) *Handshake {
	if len(b) != 68 {
		return nil
	}

	var infoHash [20]byte
	copy(infoHash[:], b[28:48])

	handshake := &Handshake{
		InfoHash: infoHash,
		PeerID:   b[48:68],
	}

	return handshake
}

func PerformHandshake(torrent *Torrent, peerIP string, peerID []byte) (*Handshake, error) {
	conn, err := net.Dial("tcp", peerIP)
	if err != nil {
		return nil, fmt.Errorf("error connecting to peer %s: %v", peerIP, err)
	}
	defer conn.Close()

	handshake := Handshake{
		PeerID:   peerID,
		InfoHash: torrent.InfoHash,
	}

	_, err = conn.Write(handshake.ToBytes())
	if err != nil {
		return nil, fmt.Errorf("error writing handshake to conn: %v", err)
	}

	response := make([]byte, 68)
	_, err = conn.Read(response)
	if err != nil {
		return nil, fmt.Errorf("error reading handshake response: %v", err)
	}

	recvHandshake := HandshakeFromBytes(response)

	return recvHandshake, nil
}
