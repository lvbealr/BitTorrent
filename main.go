package main

import (
	"BitTorrent/torrent"
	"fmt"
	"log"
	"os"
)

func main() {
	logFile, err := os.OpenFile("torrent.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v\n", err)
	}
	log.SetOutput(logFile)
	defer logFile.Close()

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

	Torrent.RefreshPeer()
	err = Torrent.StartDownload(os.Args[2])
	if err != nil {
		log.Fatalf("%v\n", err)
	}
}
