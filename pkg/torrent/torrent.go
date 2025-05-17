package torrent

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/torbenconto/pebl/pkg/bencode"
)

type Torrent struct {
	TrackerURL  string
	Length      int
	InfoHash    string
	PieceLength int
	Pieces      []string
}

func ReadMetaInfoFile(path string) (Torrent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Torrent{}, err
	}

	result, _, hash, err := bencode.DecodeWithInfoHash(data)
	if err != nil {
		return Torrent{}, err
	}

	dict, ok := result.(map[string]interface{})
	if !ok {
		return Torrent{}, err
	}

	info, ok := dict["info"].(map[string]interface{})
	if info == nil || !ok {
		return Torrent{}, fmt.Errorf("no info section")
	}

	rawPieces, ok := info["pieces"].(string)
	if !ok {
		return Torrent{}, fmt.Errorf("invalid pieces format")
	}
	pieceBytes := []byte(rawPieces)

	var pieces []string
	for i := 0; i < len(pieceBytes); i += 20 {
		end := i + 20
		if end > len(pieceBytes) {
			return Torrent{}, fmt.Errorf("invalid pieces length (not divisible by 20)")
		}
		pieces = append(pieces, fmt.Sprintf("%x", pieceBytes[i:end]))
	}

	final := Torrent{
		TrackerURL:  dict["announce"].(string),
		Length:      info["length"].(int),
		InfoHash:    fmt.Sprintf("%x", hash),
		PieceLength: info["piece length"].(int),
		Pieces:      pieces,
	}

	return final, nil
}

func DiscoverPeers(torrent Torrent, peerID string) ([]string, error) {
	infoHashBytes, err := hex.DecodeString(torrent.InfoHash)
	if err != nil {
		return nil, fmt.Errorf("invalid info hash: %v", err)
	}

	base, err := url.Parse(torrent.TrackerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid tracker URL: %v", err)
	}

	params := url.Values{
		"peer_id":    {peerID},
		"port":       {"6881"},
		"uploaded":   {"0"},
		"downloaded": {"0"},
		"left":       {strconv.Itoa(torrent.Length)},
		"compact":    {"1"},
		"event":      {"started"},
	}

	escapedInfoHash := ""
	for _, b := range infoHashBytes {
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
		return nil, fmt.Errorf("invalid tracker response")
	}

	peerBytes, ok := dict["peers"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid peers format")
	}

	var peers []string
	for i := 0; i+6 <= len(peerBytes); i += 6 {
		ip := net.IP(peerBytes[i : i+4])
		port := binary.BigEndian.Uint16([]byte(peerBytes[i+4 : i+6]))
		peers = append(peers, fmt.Sprintf("%s:%d", ip.String(), port))
	}

	return peers, nil
}
