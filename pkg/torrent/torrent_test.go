package torrent

import (
	"fmt"
	"testing"
)

func TestReadMetaInfoFile(t *testing.T) {
	torrent, _ := ReadMetaInfoFile("sample.torrent")

	resp, err := DiscoverPeers(torrent, []byte("-GT0001-123456789012"))
	if err != nil {
		panic(err)
	}

	for _, peer := range resp {
		s, err := PerformHandshake(&torrent, peer, []byte("-GT0001-123456789012"))
		if err != nil {
			panic(err)
		}

		fmt.Println(s)
	}

	fmt.Println(resp)
}
