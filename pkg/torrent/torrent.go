package torrent

import (
	"fmt"
	"os"

	"github.com/torbenconto/pebl/pkg/bencode"
)

type Torrent struct {
	TrackerURL  string
	Length      int
	InfoHash    [20]byte
	PieceLength int
	Pieces      [][]byte
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

	var pieces [][]byte
	for i := 0; i < len(pieceBytes); i += 20 {
		end := i + 20
		if end > len(pieceBytes) {
			return Torrent{}, fmt.Errorf("invalid pieces length (not divisible by 20)")
		}
		pieces = append(pieces, pieceBytes[i:end])
	}

	final := Torrent{
		TrackerURL:  dict["announce"].(string),
		Length:      info["length"].(int),
		InfoHash:    hash,
		PieceLength: info["piece length"].(int),
		Pieces:      pieces,
	}

	return final, nil
}
