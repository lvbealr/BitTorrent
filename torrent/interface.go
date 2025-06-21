package torrent

// --------------------------------------------------------------------------------------------- //

/*
SetTorrentFile loads and parses a .torrent file from the given path.
It returns a pointer to a TorrentFile struct populated with metadata.

Parameters:
  - path: Path to the .torrent file on disk.

Returns:
  - *TorrentFile: Pointer to the parsed torrent structure.
  - error: Non-nil if parsing fails.
*/
func SetTorrentFile(path string) (*TorrentFile, error) {
	var Torrent TorrentFile
	err := Parse(&Torrent, path)
	if err != nil {
		return nil, err
	}

	return &Torrent, nil
}

// --------------------------------------------------------------------------------------------- //

/*
FindConnections contacts the tracker and retrieves a list of peers.

It sends a tracker request using the given TorrentFile metadata,
then parses the compact peer list received in the response.

Parameters:
  - Torrent: Pointer to the TorrentFile for which to find peers.

Returns:
  - []Peer: List of peers extracted from the tracker response.
  - error: Non-nil if tracker communication or peer parsing fails.
*/
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
