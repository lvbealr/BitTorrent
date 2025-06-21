package torrent

import (
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"
)

// --------------------------------------------------------------------------------------------- //

func (Torrent *TorrentFile) ParsePeers(peers string) ([]Peer, error) {
	peerBytes := []byte(peers)
	if len(peerBytes)%6 != 0 {
		return nil, fmt.Errorf("Invalid peers length: %d (must be multiple of 6)\n", len(peerBytes))
	}

	var result []Peer

	for i := 0; i < len(peerBytes); i += 6 {
		ip := fmt.Sprintf("%d.%d.%d.%d", peerBytes[i], peerBytes[i+1], peerBytes[i+2], peerBytes[i+3])
		port := binary.BigEndian.Uint16(peerBytes[i+4 : i+6])
		result = append(result, Peer{IP: ip, Port: port})
	}

	return result, nil
}

// --------------------------------------------------------------------------------------------- //

func (Torrent *TorrentFile) GetInfoHash() ([20]byte, error) {
	return Torrent.Info.InfoHash, nil
}

func (Torrent *TorrentFile) GeneratePeerID() (string, error) {
	const (
		prefix       = "-GT0001-"
		peerIDLength = 20
		randomLength = peerIDLength - len(prefix)
	)

	randomBytes := make([]byte, randomLength)
	_, err := crand.Read(randomBytes)
	if err != nil {
		return "", fmt.Errorf("Generating random bytes error: %v\n", err)
	}

	chars := "0123456789abcdefghijklmnopqrstuvxyz"
	for i, b := range randomBytes {
		randomBytes[i] = chars[int(b)%len(chars)]
	}

	return prefix + string(randomBytes), nil
}

func (Torrent *TorrentFile) GetTotalSize() (uint64, error) {
	if len(Torrent.Info.Files) == 0 {
		return uint64(Torrent.Info.Length), nil
	}

	var total uint64 = 0

	for _, file := range Torrent.Info.Files {
		total += uint64(file.Length)
	}

	return total, nil
}

func isHTTP(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

func isUDP(url string) bool {
	return strings.HasPrefix(url, "udp://")
}

func (Torrent *TorrentFile) GenerateTransactionID() (uint32, error) {
	var buf [4]byte

	_, err := crand.Read(buf[:])
	if err != nil {
		return 0, fmt.Errorf("Generating transaction ID error: %v\n", err)
	}

	return binary.BigEndian.Uint32(buf[:]), nil
}

func (Torrent *TorrentFile) BuildFileInfo(outputDir string) error {
	Torrent.Files = nil

	if len(Torrent.Info.Files) == 0 {
		Torrent.Files = append(Torrent.Files, FileInfo{
			Path:   filepath.Join(outputDir, Torrent.Info.Name),
			Length: Torrent.Info.Length,
			Offset: 0,
		})
	} else {
		baseDir := filepath.Join(outputDir, Torrent.Info.Name)
		var offset int64 = 0

		for _, fileEntry := range Torrent.Info.Files {
			parts := []string{baseDir}
			parts = append(parts, fileEntry.Path...)
			fullPath := filepath.Join(parts...)

			Torrent.Files = append(Torrent.Files, FileInfo{
				Path:   fullPath,
				Length: fileEntry.Length,
				Offset: offset,
			})

			offset += fileEntry.Length
		}
	}

	return nil
}

// --------------------------------------------------------------------------------------------- //
