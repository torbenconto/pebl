package torrent

import (
	"fmt"
	"os"

	"github.com/torbenconto/pebl/pkg/bencode"
)

type Torrent struct {
	TrackerURL string
	Length     int
}

func ReadMetaInfoFile(path string) (Torrent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Torrent{}, err
	}

	result, err := bencode.Decode(data)
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

	final := Torrent{
		TrackerURL: dict["announce"].(string),
		Length:     info["length"].(int),
	}

	return final, nil
}
