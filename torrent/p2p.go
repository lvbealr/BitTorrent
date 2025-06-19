package torrent

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

type Handshake struct {
	ProtocolNameLength byte
	Protocol           [19]byte
	Reserved           [8]byte
	InfoHash           [20]byte
	PeerID             [20]byte
}

func (Torrent *TorrentFile) PerformHandshake(peer Peer) (string, error) {
	addr := fmt.Sprintf("%s:%d", peer.IP, peer.Port)

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("Connecting to peer failed: %v", err)
	}
	defer conn.Close()

	protocol := "BitTorrent protocol"

	var hs Handshake
	hs.ProtocolNameLength = byte(len(protocol))
	copy(hs.Protocol[:], protocol)
	hs.InfoHash = Torrent.Info.InfoHash

	peerID, err := Torrent.GeneratePeerID()
	if err != nil {
		return "", err
	}

	copy(hs.PeerID[:], peerID)

	fmt.Printf("Sending handshake to %s: ProtocolNameLength=%d, InfoHash=%x, PeerID=%s\n",
		addr, hs.ProtocolNameLength, hs.InfoHash, peerID)
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))

	err = binary.Write(conn, binary.BigEndian, &hs)
	if err != nil {
		return "", fmt.Errorf("Sending handshake error: %v\n", err)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var response Handshake

	err = binary.Read(conn, binary.BigEndian, &response)
	if err != nil {
		return "", fmt.Errorf("Reading handshake error: %v\n", err)
	}

	fmt.Printf("Received handshake from %s: ProtocolNameLength=%d, Protocol=%s, InfoHash=%x, PeerID=%s\n",
		addr, response.ProtocolNameLength, string(response.Protocol[:]), response.InfoHash, string(response.PeerID[:]))
	if response.ProtocolNameLength != 19 || string(response.Protocol[:]) != protocol {
		return "", fmt.Errorf("Invalid protocol in handshake\n")
	}

	if !bytes.Equal(response.InfoHash[:], Torrent.Info.InfoHash[:]) {
		return "", fmt.Errorf("Info hash mismatch in handshake\n")
	}

	remotePeerID := string(response.PeerID[:])

	return remotePeerID, nil
}
