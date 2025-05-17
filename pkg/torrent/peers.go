package torrent

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/torbenconto/pebl/pkg/bencode"
)

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
