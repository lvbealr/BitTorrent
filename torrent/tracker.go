package torrent

import (
	"encoding/binary"
	"fmt"
	mrand "math/rand"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/jackpal/bencode-go"
)

// --------------------------------------------------------------------------------------------- //

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
	defer Torrent.SendHTTPTrackerRequest(announceURL)

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

// --------------------------------------------------------------------------------------------- //
