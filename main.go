package main

import (
	"BitTorrent/torrent"
	"fmt"
	"log"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: ./BitTorrent <path-to-torrent-file>\n")
		os.Exit(1)
	}

	path := os.Args[1]
	torrent, err := torrent.Parse(path)

	response, err := torrent.SendTrackerResponse()
	if err != nil {
		log.Fatalf("%v\n", err)
	}

	peers, err := torrent.ParsePeers(response.Peers)
	if err != nil {
		log.Fatalf("%v\n", err)
	}

	for i := 0; i < len(peers); i++ {
		fmt.Println(peers[i].IP, peers[i].Port)
	}

	fmt.Printf("Interval: %d seconds\n", response.Interval)
}
