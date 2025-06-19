package torrent

// --------------------------------------------------------------------------------------------- //

func SetTorrentFile(path string) (*TorrentFile, error) {
	var Torrent TorrentFile
	err := Parse(&Torrent, path)
	if err != nil {
		return nil, err
	}

	return &Torrent, nil
}

func FindConnections(Torrent *TorrentFile) ([]Peer, error) {
	response, err := Torrent.SendTrackerResponse()
	if err != nil {
		return nil, err
	}

	allPeers, err := Torrent.ParsePeers(response.Peers)
	if err != nil {
		return nil, err
	}

	return allPeers, nil
}

// --------------------------------------------------------------------------------------------- //
