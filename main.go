package main

import (
	"BitTorrent/torrent"
	"fmt"
	"log"
	"os"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: ./BitTorrent <path-to-torrent-file> <output-path>\n")
		os.Exit(1)
	}

	Torrent, err := torrent.SetTorrentFile(os.Args[1])
	if err != nil {
		log.Fatalf("%v\n", err)
	}

	peers, err := torrent.FindConnections(Torrent)
	if err != nil {
		log.Fatalf("%v\n", err)
	}

	Torrent.ConnectToPeers(peers)

	err = Torrent.StartDownload(os.Args[2])
	if err != nil {
		log.Fatalf("%v\n", err)
	}

	fmt.Println("Download completed")
}
