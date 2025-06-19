package torrent

import (
	"log"
	"os"

	"github.com/jackpal/bencode-go"
)

func Parse(file string) *TorrentFile {
	src, err := os.Open(file)
	if err != nil {
		log.Fatalf("Opening file error: %v\n", err)
	}
	defer src.Close()

	var torrent TorrentFile

	err = bencode.Unmarshal(src, &torrent)
	if err != nil {
		log.Fatalf("Decoding error: %v\n", err)
	}

	return &torrent
}
