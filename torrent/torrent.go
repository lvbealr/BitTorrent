package torrent

import (
	mrand "math/rand"
	"time"
)

// --------------------------------------------------------------------------------------------- //

type TorrentFile struct {
	Announce     string                 `bencode:"announce"`
	AnnounceList [][]string             `bencode:"announce-list"`
	Comment      string                 `bencode:"comment"`
	CreatedBy    string                 `bencode:"created by"`
	CreationDate int64                  `bencode:"creation date"`
	Encoding     string                 `bencode:"encoding"`
	Info         TorrentInfo            `bencode:"info"`
	Nodes        [][]interface{}        `bencode:"nodes"`
	URLList      []string               `bencode:"url-list"`
	HTTPSeeds    []string               `bencode:"httpseeds"`
	Publisher    string                 `bencode:"publisher"`
	PublisherURL string                 `bencode:"publisher-url"`
	Source       string                 `bencode:"source"`
	Signature    string                 `bencode:"signature"`
	Custom       map[string]interface{} `bencode:"-"`
}

type TorrentInfo struct {
	PieceLength int64                  `bencode:"piece length"`
	Pieces      string                 `bencode:"pieces"`
	Name        string                 `bencode:"name"`
	Length      int64                  `bencode:"length"`
	Files       []TorrentFileEntry     `bencode:"files"`
	MD5Sum      string                 `bencode:"md5sum"`
	Private     int                    `bencode:"private"`
	Source      string                 `bencode:"source"`
	MetaVersion int                    `bencode:"meta version"`
	FileTree    map[string]interface{} `bencode:"file tree"`
	PieceLayers map[string]string      `bencode:"piece layers"`
	PiecesRoot  string                 `bencode:"pieces root"`
	Custom      map[string]interface{} `bencode:"-"`
	InfoHash    [20]byte               `bencode:"-"`
}

type TorrentFileEntry struct {
	Length     int64                  `bencode:"length"`
	Path       []string               `bencode:"path"`
	MD5Sum     string                 `bencode:"md5sum"`
	PiecesRoot string                 `bencode:"pieces root"`
	Custom     map[string]interface{} `bencode:"-"`
}

type TrackerResponse struct {
	Peers    string `bencode:"peers"`
	Failure  string `bencode:"failure reason"`
	Interval int    `bencode:"interval"`
}

type Peer struct {
	IP   string
	Port uint16
}

// --------------------------------------------------------------------------------------------- //

func init() {
	mrand.New(mrand.NewSource(time.Now().UnixNano()))
}

// --------------------------------------------------------------------------------------------- //
