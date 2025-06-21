package torrent

import (
	mrand "math/rand"
	"net"
	"os"
	"sync"
	"time"
)

// --------------------------------------------------------------------------------------------- //

// TorrentFile represents the full structure of a .torrent file,
// including both standard metadata and additional fields used
// by the torrent client during download.
type TorrentFile struct {
	Announce      string                 `bencode:"announce"`      // URL of the main tracker
	AnnounceList  [][]string             `bencode:"announce-list"` // List of alternative trackers (each sublist is a tier)
	Comment       string                 `bencode:"comment"`       // Optional comment about the torrent
	CreatedBy     string                 `bencode:"created by"`    // Name of the program that created the torrent
	CreationDate  int64                  `bencode:"creation date"` // Creation time (Unix timestamp)
	Encoding      string                 `bencode:"encoding"`      // Character encoding used in text fields
	Info          TorrentInfo            `bencode:"info"`          // Core metadata about the files being shared
	Nodes         [][]interface{}        `bencode:"nodes"`         // DHT bootstrap nodes (IP and port)
	URLList       []string               `bencode:"url-list"`      // List of Web Seed URLs (HTTP/FTP sources)
	HTTPSeeds     []string               `bencode:"httpseeds"`     // Legacy HTTP seed URLs
	Publisher     string                 `bencode:"publisher"`     // Name of the publisher (optional)
	PublisherURL  string                 `bencode:"publisher-url"` // URL of the publisher (optional)
	Source        string                 `bencode:"source"`        // Source identifier for private torrents
	Signature     string                 `bencode:"signature"`     // Digital signature (if present)
	Custom        map[string]interface{} `bencode:"-"`             // Non-standard/custom fields (not encoded)
	Peers         []Peer                 `bencode:"-"`             // List of peers participating in the download
	PeersMutex    sync.Mutex             `bencode:"-"`             // Mutex for synchronizing access to Peers
	PieceLength   int64                  `bencode:"-"`             // Length of each piece in bytes
	NumPieces     int                    `bencode:"-"`             // Total number of pieces
	PieceHashes   [][20]byte             `bencode:"-"`             // SHA-1 hashes of each piece
	Downloaded    []bool                 `bencode:"-"`             // Bitfield indicating downloaded pieces
	DownloadMutex sync.Mutex             `bencode:"-"`             // Mutex for synchronizing download state
	Files         []FileInfo             `bencode:"-"`             // Local file info (paths, offsets, handles)
}

// TorrentInfo represents the "info" dictionary inside a .torrent file,
// which contains the essential data about the files being shared.
type TorrentInfo struct {
	PieceLength int64                  `bencode:"piece length"` // Length of each piece in bytes
	Pieces      string                 `bencode:"pieces"`       // Concatenated SHA-1 hashes of pieces
	Name        string                 `bencode:"name"`         // Name of the file or directory
	Length      int64                  `bencode:"length"`       // Total length (for single-file torrents)
	Files       []TorrentFileEntry     `bencode:"files"`        // List of files (for multi-file torrents)
	MD5Sum      string                 `bencode:"md5sum"`       // Optional MD5 checksum of the file
	Private     int                    `bencode:"private"`      // 1 if this is a private torrent
	Source      string                 `bencode:"source"`       // Optional source field
	MetaVersion int                    `bencode:"meta version"` // BEP-9: versioning for future compatibility
	FileTree    map[string]interface{} `bencode:"file tree"`    // BEP-47: file tree representation
	PieceLayers map[string]string      `bencode:"piece layers"` // BEP-47: used for Merkle root trees
	PiecesRoot  string                 `bencode:"pieces root"`  // BEP-47: root hash of the Merkle tree
	Custom      map[string]interface{} `bencode:"-"`            // Non-standard/custom fields (not encoded)
	InfoHash    [20]byte               `bencode:"-"`            // SHA-1 hash of the bencoded Info dictionary
}

// TorrentFileEntry represents an individual file in a multi-file torrent.
type TorrentFileEntry struct {
	Length     int64                  // Length of the file in bytes
	Path       []string               // File path split into directories and file name
	MD5Sum     string                 // Optional MD5 checksum
	PiecesRoot string                 // Merkle root for this file (BEP-47)
	Custom     map[string]interface{} // Custom/non-standard fields
}

// TrackerResponse represents the response from a tracker server.
type TrackerResponse struct {
	Peers    string // Compact peer list (each peer is 6 bytes: 4 for IP, 2 for port)
	Failure  string // Error message if the tracker request failed
	Interval int    // Interval (in seconds) before the next announce request
}

// Peer represents a remote peer in the BitTorrent swarm.
type Peer struct {
	IP         string   // IP address of the peer
	Port       uint16   // Port number of the peer
	PeerID     string   // Peer ID (optional)
	Connection net.Conn // TCP connection to the peer
	Choked     bool     // Whether this peer is currently choking us
	Bitfield   []byte   // Bitfield indicating which pieces the peer has
}

// FileInfo contains information about a file on disk,
// used for reading and writing data during the download process.
type FileInfo struct {
	Path   string   // Full file path on the local filesystem
	Length int64    // Length of the file in bytes
	Offset int64    // Offset from the beginning of the torrent data
	Handle *os.File `bencode:"-"` // File handle (not part of the .torrent format)
}

// --------------------------------------------------------------------------------------------- //

// init seeds the math/rand random number generator with the current time,
// ensuring different random sequences on each run.
func init() {
	mrand.New(mrand.NewSource(time.Now().UnixNano()))
}

// --------------------------------------------------------------------------------------------- //
