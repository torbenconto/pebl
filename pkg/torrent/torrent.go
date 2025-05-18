package torrent

import (
	"fmt"
	"os"

	"github.com/torbenconto/pebl/pkg/bencode"
)

type File struct {
	Length int
	Path   []string
}

type Torrent struct {
	TrackerURL  string
	Length      int
	InfoHash    [20]byte
	PieceLength int
	Pieces      [][]byte
	Files       []File
}

func (t *Torrent) GetFiles() []File {
	if len(t.Files) > 0 {
		return t.Files
	}
	return []File{{Length: t.Length, Path: []string{"file"}}}
}

func extractTrackerURLs(dict map[string]interface{}) ([]string, error) {
	var trackers []string

	if al, ok := dict["announce-list"]; ok {
		if tiers, ok := al.([]interface{}); ok {
			for _, tier := range tiers {
				if tierList, ok := tier.([]interface{}); ok {
					for _, urlRaw := range tierList {
						if urlStr, ok := urlRaw.(string); ok {
							trackers = append(trackers, urlStr)
						}
					}
				}
			}
		}
	}

	if len(trackers) == 0 {
		if announce, ok := dict["announce"].(string); ok {
			trackers = append(trackers, announce)
		} else {
			return nil, fmt.Errorf("no announce or announce-list found")
		}
	}

	return trackers, nil
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
		return Torrent{}, fmt.Errorf("invalid torrent metadata")
	}

	info, ok := dict["info"].(map[string]interface{})
	if !ok {
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

	trackers, err := extractTrackerURLs(dict)
	if err != nil {
		return Torrent{}, err
	}

	torrent := Torrent{
		TrackerURL:  trackers[0],
		InfoHash:    hash,
		PieceLength: info["piece length"].(int),
		Pieces:      pieces,
	}

	if filesRaw, ok := info["files"]; ok {
		filesList, ok := filesRaw.([]interface{})
		if !ok {
			return Torrent{}, fmt.Errorf("invalid files format")
		}

		var files []File
		for _, f := range filesList {
			fMap, ok := f.(map[string]interface{})
			if !ok {
				return Torrent{}, fmt.Errorf("invalid file entry")
			}

			length, ok := fMap["length"].(int)
			if !ok {
				return Torrent{}, fmt.Errorf("file length missing or invalid")
			}

			pathList, ok := fMap["path"].([]interface{})
			if !ok {
				return Torrent{}, fmt.Errorf("file path missing or invalid")
			}

			var pathStrings []string
			for _, p := range pathList {
				ps, ok := p.(string)
				if !ok {
					return Torrent{}, fmt.Errorf("file path element not string")
				}
				pathStrings = append(pathStrings, ps)
			}

			files = append(files, File{
				Length: length,
				Path:   pathStrings,
			})
		}

		torrent.Files = files
	} else {
		length, ok := info["length"].(int)
		if !ok {
			return Torrent{}, fmt.Errorf("single file torrent missing length")
		}
		torrent.Length = length
	}

	return torrent, nil
}
