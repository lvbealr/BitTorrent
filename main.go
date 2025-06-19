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

	var Torrent torrent.TorrentFile
	err := torrent.Parse(&Torrent, path)
	if err != nil {
		log.Fatalf("%v\n", err)
	}

	response, err := Torrent.SendTrackerResponse()
	if err != nil {
		log.Fatalf("%v\n", err)
	}

	peers, err := Torrent.ParsePeers(response.Peers)
	if err != nil {
		log.Fatalf("%v\n", err)
	}

	for i := 0; i < len(peers); i++ {
		fmt.Println(peers[i].IP, peers[i].Port)
	}

	fmt.Printf("Interval: %d seconds\n", response.Interval)

	for i, peer := range peers {
		fmt.Printf("[%d]	%s:%d\n", i+1, peer.IP, peer.Port)
		remotePeerID, err := Torrent.PerformHandshake(peer)
		if err != nil {
			fmt.Printf("Handshake failed: %v\n", err)
			continue
		}

		fmt.Printf("Handshake successful, remotePeerID: %s\n", remotePeerID)
	}
}
