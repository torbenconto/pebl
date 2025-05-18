package torrent

import (
	"fmt"
	"testing"
	"time"
)

func TestReadMetaInfoFile(t *testing.T) {
	torrent, err := ReadMetaInfoFile("sample.torrent")
	if err != nil {
		panic(err)
	}

	peerID := []byte("-GT0001-123456789012")
	peers, err := DiscoverPeers(torrent, peerID)
	if err != nil {
		panic(err)
	}
	peerManager, _ := NewPeerManager(&torrent, "test")

	for _, peerAddr := range peers {
		peerConn, err := PerformHandshakeAndConnect(&torrent, peerAddr, peerID)
		if err != nil {
			fmt.Printf("Handshake failed with %s: %v\n", peerAddr, err)
			continue
		}

		peerManager.Add(peerConn)
		go peerManager.handlePeer(peerConn)

		interested := NewMessage(MsgInterested)
		err = peerConn.Send(interested)
		if err != nil {
			fmt.Printf("Failed to send Interested message to %s: %v\n", peerAddr, err)
		}

		select {
		case <-peerConn.UnchokeC:
			fmt.Printf("Peer %s unchoked us\n", peerAddr)

			const blockSize = 16384
			pieceIndex := uint32(0)

			pieceLength := torrent.PieceLength
			if int(pieceIndex) == len(torrent.Pieces)-1 {
				totalLength := torrent.Length
				lastPieceLength := totalLength - (len(torrent.Pieces)-1)*pieceLength
				pieceLength = lastPieceLength
			}

			for begin := uint32(0); begin < uint32(pieceLength); begin += blockSize {
				var reqLength uint32 = blockSize
				if begin+blockSize > uint32(pieceLength) {
					reqLength = uint32(pieceLength) - begin
				}
				request := NewRequestMessage(pieceIndex, begin, reqLength)
				err := peerConn.Send(request)
				if err != nil {
					fmt.Printf("Failed to send Request message to %s: %v\n", peerAddr, err)
					break
				}
				fmt.Printf("Sent Request: piece %d, begin %d, length %d to %s\n",
					pieceIndex, begin, reqLength, peerAddr)

			}

		case <-time.After(10 * time.Second):
			fmt.Printf("Timeout waiting for unchoke from %s\n", peerAddr)
		}
	}

}
