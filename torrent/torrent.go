package torrent

import (
	"bytes"
	crand "crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	mrand "math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackpal/bencode-go"
)

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

func (Torrent *TorrentFile) GetInfoHash() ([]byte, error) {
	var infoBytes bytes.Buffer
	err := bencode.Marshal(&infoBytes, Torrent.Info)
	if err != nil {
		return nil, fmt.Errorf("Marshaling error: %v\n", err)
	}

	hash := sha1.Sum(infoBytes.Bytes())
	return hash[:], nil
}

func (Torrent *TorrentFile) GeneratePeerID() (string, error) {
	const (
		prefix       = "-GT0001-"
		peerIDLength = 20
		randomLength = peerIDLength - len(prefix)
	)

	randomBytes := make([]byte, randomLength)
	_, err := crand.Read(randomBytes)
	if err != nil {
		return "", fmt.Errorf("Generating random bytes error: %v\n", err)
	}

	chars := "0123456789abcdefghijklmnopqrstuvxyz"
	for i, b := range randomBytes {
		randomBytes[i] = chars[int(b)%len(chars)]
	}

	return prefix + string(randomBytes), nil
}

func (Torrent *TorrentFile) GetTotalSize() (uint64, error) {
	if len(Torrent.Info.Files) == 0 {
		return uint64(Torrent.Info.Length), nil
	}

	var total uint64 = 0

	for _, file := range Torrent.Info.Files {
		total += uint64(file.Length)
	}

	return total, nil
}

func isHTTP(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

func isUDP(url string) bool {
	return strings.HasPrefix(url, "udp://")
}

func (Torrent *TorrentFile) SendHTTPTrackerRequest(announceURL string) (*TrackerResponse, error) {
	u, err := url.Parse(announceURL)
	if err != nil {
		return nil, fmt.Errorf("URL parsing error: %v\n", err)
	}

	infoHash, err := Torrent.GetInfoHash()
	if err != nil {
		return nil, err
	}

	peerID, err := Torrent.GeneratePeerID()
	if err != nil {
		return nil, err
	}

	left, err := Torrent.GetTotalSize()
	if err != nil {
		return nil, err
	}

	params := url.Values{}
	params.Add("info_hash", string(infoHash))
	params.Add("peer_id", peerID)
	params.Add("port", "6881")
	params.Add("uploaded", "0")
	params.Add("downloaded", "0")
	params.Add("left", fmt.Sprintf("%d", left))
	params.Add("compact", "1")
	params.Add("event", "started")

	u.RawQuery = params.Encode()

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	response, err := client.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("Sending response error: %v\n", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Tracker status code error: %v\n", err)
	}

	var trackerResp TrackerResponse
	err = bencode.Unmarshal(response.Body, &trackerResp)
	if err != nil {
		return nil, fmt.Errorf("Decoding tracker response error: %v\n", err)
	}

	return &trackerResp, nil
}

func (Torrent *TorrentFile) CreateAnnounceRequest(
	connectionID uint64,
	action uint32,
	transactionID uint32,
	infoHash []byte,
	peerID string,
	downloaded uint64,
	left uint64,
	uploaded uint64,
	event uint32,
	IP uint32,
	key uint32,
	num_want int32,
	port uint16) []byte {

	announceReq := make([]byte, 98)

	binary.BigEndian.PutUint64(announceReq[0:8], connectionID)
	binary.BigEndian.PutUint32(announceReq[8:12], action)
	binary.BigEndian.PutUint32(announceReq[12:16], transactionID)

	copy(announceReq[16:36], infoHash)
	copy(announceReq[36:56], []byte(peerID))

	binary.BigEndian.PutUint64(announceReq[56:64], downloaded)
	binary.BigEndian.PutUint64(announceReq[64:72], left)
	binary.BigEndian.PutUint64(announceReq[72:80], uploaded)

	binary.BigEndian.PutUint32(announceReq[80:84], event)
	binary.BigEndian.PutUint32(announceReq[88:92], key)
	binary.BigEndian.PutUint32(announceReq[92:96], uint32(num_want))
	binary.BigEndian.PutUint16(announceReq[96:98], port)

	return announceReq
}

func (Torrent *TorrentFile) GenerateTransactionID() (uint32, error) {
	var buf [4]byte

	_, err := crand.Read(buf[:])
	if err != nil {
		return 0, fmt.Errorf("Generating transaction ID error: %v\n", err)
	}

	return binary.BigEndian.Uint32(buf[:]), nil
}

func (Torrent *TorrentFile) SendUDPTrackerRequest(announceURL string) (*TrackerResponse, error) {
	u, err := url.Parse(announceURL)
	if err != nil {
		return nil, fmt.Errorf("Parsing UDP URL error: %v\n", err)
	}

	addr, err := net.ResolveUDPAddr("udp", u.Host)
	if err != nil {
		return nil, fmt.Errorf("Resolving UDP address error: %v\n", err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, fmt.Errorf("Dial UDP error: %v\n", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	transactionID, err := Torrent.GenerateTransactionID()
	if err != nil {
		return nil, err
	}

	connectReq := make([]byte, 16)

	binary.BigEndian.PutUint64(connectReq[0:8], 0x41727101980) // Protocol ID
	binary.BigEndian.PutUint32(connectReq[8:12], 0)            // Action: Connect
	binary.BigEndian.PutUint32(connectReq[12:16], transactionID)

	_, err = conn.Write(connectReq)
	if err != nil {
		return nil, fmt.Errorf("Sending connect request error: %v\n", err)
	}

	resp := make([]byte, 16)
	n, err := conn.Read(resp)
	if err != nil {
		return nil, fmt.Errorf("Reading connect response error: %v\n", err)
	}

	if n < 16 {
		return nil, fmt.Errorf("Invalid connect response length: %d\n", n)
	}

	action := binary.BigEndian.Uint32(resp[0:4])
	if action != 0 {
		return nil, fmt.Errorf("Invalid connect action: %d\n", action)
	}

	if binary.BigEndian.Uint32(resp[4:8]) != transactionID {
		return nil, fmt.Errorf("Transaction ID mismatch\n")
	}

	connectionID := binary.BigEndian.Uint64(resp[8:16])

	infoHash, err := Torrent.GetInfoHash()
	if err != nil {
		return nil, err
	}

	peerID, err := Torrent.GeneratePeerID()
	if err != nil {
		return nil, err
	}

	left, err := Torrent.GetTotalSize()
	if err != nil {
		return nil, err
	}

	const (
		ANNOUNCE   = 1
		DOWNLOADED = 0
		UPLOADED   = 0
		STARTED    = 2
		IP         = 0
		NUM_WANT   = -1
		PORT       = 6881
	)

	announceReq := Torrent.CreateAnnounceRequest(
		connectionID,
		ANNOUNCE,
		transactionID,
		infoHash,
		peerID,
		DOWNLOADED,
		left,
		UPLOADED,
		STARTED,
		IP,
		mrand.Uint32(),
		NUM_WANT,
		PORT,
	)

	_, err = conn.Write(announceReq)
	if err != nil {
		return nil, fmt.Errorf("Sending announce request error: %v\n", err)
	}

	resp = make([]byte, 1024)
	n, err = conn.Read(resp)
	if err != nil {
		return nil, fmt.Errorf("Reading announce response error: %v", err)
	}

	if n < 20 {
		return nil, fmt.Errorf("Invalid announce response length: %d", n)
	}

	action = binary.BigEndian.Uint32(resp[0:4])

	if action == 3 { // error
		errorMsg := string(resp[8:n])
		return nil, fmt.Errorf("Tracker error: %s", errorMsg)
	}

	if action != 1 {
		return nil, fmt.Errorf("Invalid announce action: %d", action)
	}

	if binary.BigEndian.Uint32(resp[4:8]) != transactionID {
		return nil, fmt.Errorf("Transaction ID mismatch")
	}

	interval := int(binary.BigEndian.Uint32(resp[8:12]))

	peers := resp[20:n]
	if len(peers)%6 != 0 {
		return nil, fmt.Errorf("Invalid peers length: %d (must be multiple of 6)", len(peers))
	}

	return &TrackerResponse{
		Peers:    string(peers),
		Interval: interval,
	}, nil
}

func (Torrent *TorrentFile) SendTrackerResponse() (*TrackerResponse, error) {
	if isHTTP(Torrent.Announce) {
		return Torrent.SendHTTPTrackerRequest(Torrent.Announce)
	} else if isUDP(Torrent.Announce) {
		return Torrent.SendUDPTrackerRequest(Torrent.Announce)
	}

	for _, tier := range Torrent.AnnounceList {
		for _, announce := range tier {
			if isHTTP(announce) {
				return Torrent.SendHTTPTrackerRequest(announce)
			} else if isUDP(announce) {
				return Torrent.SendUDPTrackerRequest(announce)
			}
		}
	}

	return nil, fmt.Errorf("No supported trackers found (HTTP/UDP)\n")
}
