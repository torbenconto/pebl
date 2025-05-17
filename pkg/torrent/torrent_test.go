package torrent

import (
	"fmt"
	"testing"
)

func TestReadMetaInfoFile(t *testing.T) {
	fmt.Println(ReadMetaInfoFile("sample.torrent"))
}
