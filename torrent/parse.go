package torrent

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"os"
	"strconv"

	"github.com/jackpal/bencode-go"
)

func extractInfoBytes(data []byte) ([]byte, error) {
	idx := bytes.Index(data, []byte("4:info"))
	if idx < 0 {
		return nil, fmt.Errorf("torrent: no \"4:info\" prefix found")
	}

	start := idx + len("4:info")

	depth := 0
	for i := start; i < len(data); i++ {
		b := data[i]

		switch b {
		case 'd', 'l':
			depth++
		case 'e':
			depth--

			if depth == 0 {
				return data[start : i+1], nil
			}

		case 'i':
			j := i + 1
			for ; j < len(data) && data[j] != 'e'; j++ {
			}

			if j >= len(data) {
				return nil, fmt.Errorf("torrent: unterminated integer at %d", i)
			}

			i = j

		default:
			if b >= '0' && b <= '9' {
				j := i

				for ; j < len(data) && data[j] >= '0' && data[j] <= '9'; j++ {
				}

				if j < len(data) && data[j] == ':' {
					length, err := strconv.Atoi(string(data[i:j]))
					if err != nil {
						return nil, fmt.Errorf("Torrent: invalid string length at %d-%d", i, j)
					}

					j++

					i = j + length - 1
				}
			}
		}
	}
	return nil, fmt.Errorf("Torrent: unterminated info dict")
}

func computeInfoHash(path string) ([20]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return [20]byte{}, fmt.Errorf("Cannot read %q: %w", path, err)
	}

	infoBytes, err := extractInfoBytes(data)
	if err != nil {
		return [20]byte{}, fmt.Errorf("ExtractInfoBytes: %w", err)
	}

	return sha1.Sum(infoBytes), nil
}

// --------------------------------------------------------------------------------------------- //

func Parse(Torrent *TorrentFile, file string) error {
	src, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("Opening file error: %v\n", err)
	}
	defer src.Close()

	err = bencode.Unmarshal(src, Torrent)
	if err != nil {
		return fmt.Errorf("Decoding error: %v\n", err)
	}

	hash, err := computeInfoHash(file)
	fmt.Printf("Info hash: %x\n", hash)
	Torrent.Info.InfoHash = hash

	fmt.Printf("Parsed torrent: %s, InfoHash: %x, Computed Hash: %x\n",
		Torrent.Info.Name, Torrent.Info.InfoHash, hash)

	return nil
}

// --------------------------------------------------------------------------------------------- //
