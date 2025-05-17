package torrent

import (
	"fmt"
	"testing"
)

func TestReadMetaInfoFile(t *testing.T) {
	torrent, _ := ReadMetaInfoFile("sample.torrent")

	resp, err := DiscoverPeers(torrent, "-GT0001-123456789012")
	if err != nil {
		panic(err)
	}

	fmt.Println(resp)
}
