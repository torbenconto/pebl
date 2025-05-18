package torrent

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/torbenconto/pebl/pkg/bencode"
)

const (
	MsgChoke         = 0
	MsgUnchoke       = 1
	MsgInterested    = 2
	MsgNotInterested = 3
	MsgHave          = 4
	MsgBitfield      = 5
	MsgRequest       = 6
	MsgPiece         = 7
	MsgCancel        = 8

	blockSize = 16384 // 16KiB blocks
)

type Message struct {
	ID      uint8
	Payload []byte
}

func (m *Message) Serialize() []byte {
	length := uint32(len(m.Payload) + 1)
	buf := make([]byte, 4+1+len(m.Payload))
	binary.BigEndian.PutUint32(buf[0:4], length)
	buf[4] = m.ID
	copy(buf[5:], m.Payload)
	return buf
}

func ReadMessage(conn net.Conn) (*Message, error) {
	lengthBuf := make([]byte, 4)
	_, err := io.ReadFull(conn, lengthBuf)
	if err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(lengthBuf)
	if length == 0 {
		return nil, nil // keep-alive
	}

	msgBuf := make([]byte, length)
	_, err = io.ReadFull(conn, msgBuf)
	if err != nil {
		return nil, err
	}

	return &Message{
		ID:      msgBuf[0],
		Payload: msgBuf[1:],
	}, nil
}

func NewMessage(id uint8) *Message {
	return &Message{ID: id}
}

func NewRequestMessage(index, begin, length uint32) *Message {
	payload := make([]byte, 12)
	binary.BigEndian.PutUint32(payload[0:4], index)
	binary.BigEndian.PutUint32(payload[4:8], begin)
	binary.BigEndian.PutUint32(payload[8:12], length)

	return &Message{
		ID:      MsgRequest,
		Payload: payload,
	}
}

type PeerConn struct {
	Conn     net.Conn
	PeerID   [20]byte
	Bitfield []byte
	Choked   bool
	UnchokeC chan struct{}
	mu       sync.Mutex
	unchoked bool
}

func (p *PeerConn) Send(msg *Message) error {
	_, err := p.Conn.Write(msg.Serialize())
	return err
}

func (p *PeerConn) SetUnchoked() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.unchoked {
		p.unchoked = true
		select {
		case p.UnchokeC <- struct{}{}:
		default:
		}
	}
}

func (p *PeerConn) IsUnchoked() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.unchoked
}

type PieceBuffer struct {
	data   []byte
	bitmap []bool
	mu     sync.Mutex
}

func (pb *PieceBuffer) markBlockReceived(begin uint32, blockLen int) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	startBlock := begin / blockSize
	endBlock := (begin + uint32(blockLen) - 1) / blockSize
	for i := startBlock; i <= endBlock; i++ {
		pb.bitmap[i] = true
	}
}

func (pb *PieceBuffer) isComplete() bool {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	for _, received := range pb.bitmap {
		if !received {
			return false
		}
	}
	return true
}

type FileEntry struct {
	Path   string
	Length int64
	file   *os.File
}

type PeerManager struct {
	peers        []*PeerConn
	mu           sync.Mutex
	torrent      *Torrent
	pieceBuffers map[uint32]*PieceBuffer

	files   []FileEntry
	rootDir string

	fileMu sync.Mutex
}

func NewPeerManager(torrent *Torrent, rootDir string) (*PeerManager, error) {
	pm := &PeerManager{
		peers:        make([]*PeerConn, 0),
		torrent:      torrent,
		pieceBuffers: make(map[uint32]*PieceBuffer),
		rootDir:      rootDir,
	}

	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return nil, err
	}

	files := torrent.Files
	if len(files) == 0 {
		fileName := "file"
		files = []File{{Length: torrent.Length, Path: []string{fileName}}}
	}

	for _, fileInfo := range files {
		fullPath := filepath.Join(append([]string{rootDir}, fileInfo.Path...)...)

		dir := filepath.Dir(fullPath)

		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}

		f, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}

		pm.files = append(pm.files, FileEntry{
			Path:   strings.Join(fileInfo.Path, "/"),
			Length: int64(fileInfo.Length),
			file:   f,
		})
	}

	return pm, nil
}

func (pm *PeerManager) getOrCreatePieceBuffer(index uint32) *PieceBuffer {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pb, ok := pm.pieceBuffers[index]
	if !ok {
		size := pm.torrent.PieceLength
		if int(index) == len(pm.torrent.Pieces)-1 {
			size = pm.torrent.Length - int(index)*pm.torrent.PieceLength
		}
		pb = &PieceBuffer{
			data:   make([]byte, size),
			bitmap: make([]bool, (size+blockSize-1)/blockSize),
		}
		pm.pieceBuffers[index] = pb
	}
	return pb
}

func (pm *PeerManager) handlePieceMessage(index, begin uint32, block []byte, peer *PeerConn) {
	pb := pm.getOrCreatePieceBuffer(index)
	pb.mu.Lock()
	copy(pb.data[begin:], block)
	pb.mu.Unlock()
	pb.markBlockReceived(begin, len(block))

	if pb.isComplete() {
		hash := sha1.Sum(pb.data)
		expected := pm.torrent.Pieces[index]
		if !bytes.Equal(hash[:], expected[:]) {
			fmt.Printf("Piece %d hash mismatch! Discarding piece.\n", index)
			pb.mu.Lock()
			for i := range pb.bitmap {
				pb.bitmap[i] = false
			}
			pb.mu.Unlock()
			return
		}
		fmt.Printf("Piece %d verified, writing directly to files\n", index)

		start := int64(index) * int64(pm.torrent.PieceLength)

		err := pm.writePieceDataToFiles(start, pb.data)
		if err != nil {
			fmt.Printf("Error writing piece %d to files: %v\n", index, err)
			return
		}

		pm.mu.Lock()
		delete(pm.pieceBuffers, index)
		pm.mu.Unlock()

		payload := make([]byte, 4)
		binary.BigEndian.PutUint32(payload, index)
		pm.Broadcast(&Message{ID: MsgHave, Payload: payload})
	}
}

func (pm *PeerManager) writePieceDataToFiles(offset int64, data []byte) error {
	remaining := len(data)
	dataOffset := 0
	currentOffset := offset

	for _, f := range pm.files {
		if currentOffset >= f.Length {
			currentOffset -= f.Length
			continue
		}

		writeLen := int(f.Length - currentOffset)
		if writeLen > remaining {
			writeLen = remaining
		}

		pm.fileMu.Lock()
		n, err := f.file.WriteAt(data[dataOffset:dataOffset+writeLen], currentOffset)
		pm.fileMu.Unlock()

		if err != nil {
			return err
		}
		if n != writeLen {
			return fmt.Errorf("short write on file %s", f.Path)
		}

		remaining -= writeLen
		dataOffset += writeLen
		currentOffset = 0
		if remaining == 0 {
			break
		}
	}

	if remaining > 0 {
		return fmt.Errorf("writePieceDataToFiles: data exceeds torrent size")
	}

	return nil
}

func (pm *PeerManager) Broadcast(msg *Message) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, peer := range pm.peers {
		peer.Conn.Write(msg.Serialize())
	}
}

func (pm *PeerManager) Add(peer *PeerConn) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.peers = append(pm.peers, peer)
}

func (pm *PeerManager) Remove(peer *PeerConn) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for i, p := range pm.peers {
		if p == peer {
			p.Conn.Close()
			pm.peers = append(pm.peers[:i], pm.peers[i+1:]...)
			break
		}
	}
}

func (pm *PeerManager) handlePeer(peer *PeerConn) {
	defer pm.Remove(peer)

	for {
		msg, err := ReadMessage(peer.Conn)
		if err != nil {
			fmt.Println("error reading message:", err)
			return
		}
		if msg == nil {
			continue // keep-alive
		}

		switch msg.ID {
		case MsgChoke:
			peer.Choked = true
		case MsgUnchoke:
			peer.Choked = false
			peer.SetUnchoked()
			go pm.requestPiecesFromPeer(peer)
		case MsgBitfield:
			peer.Bitfield = msg.Payload
		case MsgHave:
			index := binary.BigEndian.Uint32(msg.Payload)
			fmt.Printf("Peer has piece %d\n", index)
		case MsgPiece:
			if len(msg.Payload) < 8 {
				fmt.Println("invalid piece message payload length")
				continue
			}
			index := binary.BigEndian.Uint32(msg.Payload[0:4])
			begin := binary.BigEndian.Uint32(msg.Payload[4:8])
			block := msg.Payload[8:]

			if int(index) >= len(pm.torrent.Pieces) {
				fmt.Printf("invalid piece index %d\n", index)
				continue
			}

			pm.handlePieceMessage(index, begin, block, peer)

		default:
			fmt.Printf("Received message ID %d\n", msg.ID)
		}
	}
}

func (pm *PeerManager) requestPiecesFromPeer(peer *PeerConn) {
	for index := uint32(0); index < uint32(len(pm.torrent.Pieces)); index++ {
		if peer.Choked {
			return
		}
		if !hasPiece(peer.Bitfield, index) {
			continue
		}

		pieceLength := pm.torrent.PieceLength
		if int(index) == len(pm.torrent.Pieces)-1 {
			pieceLength = pm.torrent.Length - int(index)*pm.torrent.PieceLength
		}

		for begin := uint32(0); begin < uint32(pieceLength); begin += blockSize {
			reqLen := blockSize
			if begin+uint32(reqLen) > uint32(pieceLength) {
				reqLen = pieceLength - int(begin)
			}
			req := NewRequestMessage(index, begin, uint32(reqLen))
			if err := peer.Send(req); err != nil {
				fmt.Println("Failed to send request:", err)
				return
			}
		}
	}
}

func hasPiece(bitfield []byte, index uint32) bool {
	byteIndex := index / 8
	bitIndex := index % 8
	if int(byteIndex) >= len(bitfield) {
		return false
	}
	return bitfield[byteIndex]&(1<<(7-bitIndex)) != 0
}

func DiscoverPeers(torrent Torrent, peerID []byte) ([]string, error) {
	base, err := url.Parse(torrent.TrackerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid tracker URL: %v", err)
	}

	params := url.Values{
		"peer_id":    {string(peerID)},
		"port":       {"6881"},
		"uploaded":   {"0"},
		"downloaded": {"0"},
		"left":       {strconv.Itoa(torrent.Length)},
		"compact":    {"1"},
		"event":      {"started"},
	}

	escapedInfoHash := ""
	for _, b := range torrent.InfoHash {
		escapedInfoHash += fmt.Sprintf("%%%02X", b)
	}

	query := "info_hash=" + escapedInfoHash + "&" + params.Encode()
	finalURL := base.Scheme + "://" + base.Host + base.Path + "?" + query

	resp, err := http.Get(finalURL)
	if err != nil {
		return nil, fmt.Errorf("tracker request failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	decoded, err := bencode.Decode(body)
	if err != nil {
		return nil, err
	}

	dict, ok := decoded.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("tracker response invalid format")
	}
	peersData, ok := dict["peers"]
	if !ok {
		return nil, fmt.Errorf("tracker response missing peers")
	}

	peersBinary, ok := peersData.(string)
	if !ok {
		return nil, fmt.Errorf("tracker peers not a string")
	}

	var peers []string
	peerBytes := []byte(peersBinary)

	for i := 0; i+6 <= len(peerBytes); i += 6 {
		ip := net.IP(peerBytes[i : i+4])
		port := binary.BigEndian.Uint16(peerBytes[i+4 : i+6])
		peers = append(peers, fmt.Sprintf("%s:%d", ip.String(), port))
	}

	return peers, nil

}
