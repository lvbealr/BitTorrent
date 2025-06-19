package main

import (
	"BitTorrent/torrent"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: ./BitTorrent <path-to-torrent-file>\n")
		os.Exit(1)
	}

	path := os.Args[1]
	torrent := torrent.Parse(path)
}
