package torrent

import (
	"fmt"
	"os"

	"github.com/jackpal/bencode-go"
)

func Parse(file string) (*TorrentFile, error) {
	src, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("Opening file error: %v\n", err)
	}
	defer src.Close()

	var torrent TorrentFile

	err = bencode.Unmarshal(src, &torrent)
	if err != nil {
		return nil, fmt.Errorf("Decoding error: %v\n", err)
	}

	return &torrent, nil
}
