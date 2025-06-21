package torrent

import (
	"encoding/binary"
	"fmt"
	"log"
	mrand "math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackpal/bencode-go"
)

// --------------------------------------------------------------------------------------------- //

/*
SendHTTPTrackerRequest sends an HTTP request to a tracker to retrieve peer information.
It constructs a request with torrent metadata and parses the bencoded response.

Parameters:
  - Torrent: Pointer to the TorrentFile containing metadata such as InfoHash and total size.
  - announceURL: URL of the HTTP tracker to contact.

Returns:
  - *TrackerResponse: Pointer to the TrackerResponse containing peers and interval.
  - error: Non-nil if URL parsing, metadata retrieval, HTTP request, or response decoding fails.
*/
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
	params.Add("info_hash", url.QueryEscape(string(infoHash[:])))
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

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("Creating HTTP request error: %v\n", err)
	}

	req.Header.Set("User-Agent", "BitTorrent/1.0")

	log.Printf("[INFO]\tSending HTTP request to %s\n", u.String())

	response, err := client.Do(req)
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

	if trackerResp.Failure != "" {
		return nil, fmt.Errorf("Tracker failure: %s\n", trackerResp.Failure)
	}

	return &trackerResp, nil
}

// --------------------------------------------------------------------------------------------- //

/*
CreateAnnounceRequest constructs a binary announce request for a UDP tracker.
It formats the request according to the BitTorrent UDP tracker protocol.

Parameters:
  - Torrent: Pointer to the TorrentFile (implicitly used for method context).
  - connectionID: Unique identifier for the UDP connection.
  - action: Action code (e.g., 1 for announce).
  - transactionID: Unique identifier for the transaction.
  - infoHash: 20-byte SHA-1 hash of the torrent's info dictionary.
  - peerID: Unique identifier for the client.
  - downloaded: Number of bytes downloaded.
  - left: Number of bytes remaining to download.
  - uploaded: Number of bytes uploaded.
  - event: Event code (e.g., 2 for started).
  - IP: IP address (0 for default).
  - key: Random key for the request.
  - num_want: Number of peers requested (-1 for default).
  - port: Port number for the client.

Returns:
  - []byte: 98-byte binary announce request.
*/
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

// --------------------------------------------------------------------------------------------- //

/*
SendUDPTrackerRequest sends a UDP request to a tracker to retrieve peer information.
It performs a connect request followed by an announce request, handling retries and response validation.

Parameters:
  - Torrent: Pointer to the TorrentFile containing metadata such as InfoHash and total size.
  - announceURL: URL of the UDP tracker to contact.

Returns:
  - *TrackerResponse: Pointer to the TrackerResponse containing peers, interval, leechers, and seeders.
  - error: Non-nil if URL parsing, connection, request sending, or response validation fails.
*/
func (Torrent *TorrentFile) SendUDPTrackerRequest(announceURL string) (*TrackerResponse, error) {
	u, err := url.Parse(announceURL)
	if err != nil {
		return nil, fmt.Errorf("parsing UDP URL error: %v", err)
	}

	addr, err := net.ResolveUDPAddr("udp", u.Host)
	if err != nil {
		return nil, fmt.Errorf("resolving UDP address error: %v", err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, fmt.Errorf("dial UDP error: %v", err)
	}
	defer conn.Close()

	transactionID, err := Torrent.GenerateTransactionID()
	if err != nil {
		return nil, err
	}

	const (
		protocolID  = 0x41727101980
		connPackage = 0x00
	)

	connectReq := make([]byte, 16)
	binary.BigEndian.PutUint64(connectReq[0:8], protocolID)
	binary.BigEndian.PutUint32(connectReq[8:12], connPackage)
	binary.BigEndian.PutUint32(connectReq[12:16], transactionID)

	log.Printf("[INFO]\tSending Connect to %s, transaction_id: %d\n", addr, transactionID)

	for attempt := 0; attempt < 3; attempt++ {
		conn.SetDeadline(time.Now().Add(time.Duration(5+attempt*2) * time.Second))
		_, err = conn.Write(connectReq)

		if err != nil {
			log.Printf("[FAIL]\tAttempt %d failed to send connect: %v\n", attempt+1, err)
			continue
		}

		resp := make([]byte, 16)

		n, err := conn.Read(resp)
		if err != nil {
			log.Printf("[FAIL]\tAttempt %d failed to read connect response: %v\n", attempt+1, err)
			continue
		}

		if n < 16 {
			log.Printf("[ERROR]\tAttempt %d invalid connect response length: %d\n", attempt+1, n)
			continue
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

		log.Printf("[INFO]\tUDP InfoHash: %x\n", infoHash)

		peerID, err := Torrent.GeneratePeerID()
		if err != nil {
			return nil, err
		}

		left, err := Torrent.GetTotalSize()
		if err != nil {
			return nil, err
		}

		const (
			announce   = 1
			downloaded = 0
			uploaded   = 0
			started    = 2
			ip         = 0
			num_want   = -1
			port       = 6881
		)

		announceReq := Torrent.CreateAnnounceRequest(
			connectionID,
			announce,
			transactionID,
			infoHash[:],
			peerID,
			downloaded,
			left,
			uploaded,
			started,
			ip,
			mrand.Uint32(),
			num_want,
			port,
		)

		log.Printf("[INFO]\tSending Announce to %s: info_hash = %x, peer_id = %s, left = %d\n", addr, infoHash, peerID, left)
		conn.SetDeadline(time.Now().Add(5 * time.Second))

		_, err = conn.Write(announceReq)
		if err != nil {
			return nil, fmt.Errorf("Sending announce request error: %v\n", err)
		}

		resp = make([]byte, 1024)

		n, err = conn.Read(resp)
		if err != nil {
			return nil, fmt.Errorf("Reading announce response error: %v\n", err)
		}

		if n < 20 {
			return nil, fmt.Errorf("Invalid announce response length: %d\n", n)
		}

		log.Printf("[INFO]\tRaw announce response: %x\n", resp[:n])
		action = binary.BigEndian.Uint32(resp[0:4])

		if action == 3 {
			errorMsg := string(resp[8:n])
			return nil, fmt.Errorf("Tracker error: %s\n", errorMsg)
		}

		if action != 1 {
			return nil, fmt.Errorf("Invalid announce action: %d\n", action)
		}

		if binary.BigEndian.Uint32(resp[4:8]) != transactionID {
			return nil, fmt.Errorf("Transaction ID mismatch\n")
		}

		interval := int(binary.BigEndian.Uint32(resp[8:12]))
		leechers := binary.BigEndian.Uint32(resp[12:16])
		seeders := binary.BigEndian.Uint32(resp[16:20])

		peers := resp[20:n]
		log.Printf("[INFO]\tRaw peers bytes: %x\n", peers)

		if len(peers)%6 != 0 {
			return nil, fmt.Errorf("Invalid peers length: %d (must be multiple of 6)\n", len(peers))
		}

		log.Printf("[INFO]\tReceived %d peers, leechers: %d, seeders: %d\n", len(peers)/6, leechers, seeders)

		trackerResp := &TrackerResponse{
			Peers:    string(peers),
			Interval: interval,
		}

		if trackerResp.Failure != "" {
			return nil, fmt.Errorf("Tracker failure: %s\n", trackerResp.Failure)
		}

		return trackerResp, nil
	}

	return nil, fmt.Errorf("No connect response after 3 attempts\n")
}

// --------------------------------------------------------------------------------------------- //

/*
SendTrackerResponse aggregates peer information from multiple trackers.
It contacts both HTTP and UDP trackers, combining their peer lists and selecting the shortest interval.

Parameters:
  - Torrent: Pointer to the TorrentFile containing tracker URLs and metadata.

Returns:
  - *TrackerResponse: Pointer to the TrackerResponse with a combined peer list and minimum interval.
  - error: Non-nil if no trackers are found or no peers are received.
*/
func (Torrent *TorrentFile) SendTrackerResponse() (*TrackerResponse, error) {
	publicTrackers := []string{
		"udp://tracker.opentrackr.org:1337/announce",
		"udp://tracker.torrent.eu.org:451/announce",
		"udp://open.tracker.cl:1337/announce",
		"udp://open.stealth.si:80/announce",
		"udp://tracker.tiny-vps.com:6969/announce",
	}

	trackersMap := make(map[string]struct{})
	if Torrent.Announce != "" {
		trackersMap[Torrent.Announce] = struct{}{}
	}

	for _, tier := range Torrent.AnnounceList {
		for _, announce := range tier {
			if announce != "" {
				trackersMap[announce] = struct{}{}
			}
		}
	}

	for _, tracker := range publicTrackers {
		trackersMap[tracker] = struct{}{}
	}

	trackers := make([]string, 0, len(trackersMap))
	for tracker := range trackersMap {
		trackers = append(trackers, tracker)
	}

	if len(trackers) == 0 {
		return nil, fmt.Errorf("No trackers found")
	}

	udpTrackers := []string{}
	httpTrackers := []string{}
	for _, tracker := range trackers {
		if isUDP(tracker) {
			udpTrackers = append(udpTrackers, tracker)
		} else if isHTTP(tracker) {
			httpTrackers = append(httpTrackers, tracker)
		}
	}

	log.Printf("[INFO]\tFound %d unique trackers: %v\n", len(trackers), trackers)
	log.Printf("[INFO]\tUDP trackers: %v\n", udpTrackers)
	log.Printf("[INFO]\tHTTP trackers: %v\n", httpTrackers)

	allPeers := make(map[string]struct{})
	var finalInterval int

	for _, announce := range udpTrackers {
		log.Printf("[INFO]\tTrying tracker: %s\n", announce)
		resp, err := Torrent.SendUDPTrackerRequest(announce)
		if err == nil {
			log.Printf("[INFO]\tSuccess from UDP tracker %s: %d peers, interval: %d\n", announce, len(resp.Peers)/6, resp.Interval)
			peers, err := Torrent.ParsePeers(resp.Peers)

			if err != nil {
				log.Printf("[FAIL]\tFailed to parse peers from %s: %v\n", announce, err)
				continue
			}

			for _, peer := range peers {
				addr := fmt.Sprintf("%s:%d", peer.IP, peer.Port)
				allPeers[addr] = struct{}{}
			}

			if finalInterval == 0 || resp.Interval < finalInterval {
				finalInterval = resp.Interval
			}

		} else {
			log.Printf("[FAIL]\tUDP tracker %s failed: %v\n", announce, err)
		}
	}

	for _, announce := range httpTrackers {
		log.Printf("[INFO]\tTrying tracker: %s\n", announce)
		resp, err := Torrent.SendHTTPTrackerRequest(announce)

		if err == nil {
			log.Printf("[INFO]\tSuccess from HTTP tracker %s: %d peers, interval: %d\n", announce, len(resp.Peers)/6, resp.Interval)
			peers, err := Torrent.ParsePeers(resp.Peers)

			if err != nil {
				log.Printf("[FAIL]\tFailed to parse peers from %s: %v\n", announce, err)
				continue
			}

			for _, peer := range peers {
				addr := fmt.Sprintf("%s:%d", peer.IP, peer.Port)
				allPeers[addr] = struct{}{}
			}

			if finalInterval == 0 || resp.Interval < finalInterval {
				finalInterval = resp.Interval
			}
		} else {
			log.Printf("[FAIL]\tHTTP tracker %s failed: %v\n", announce, err)
		}
	}

	if len(allPeers) == 0 {
		return nil, fmt.Errorf("No peers received from any tracker")
	}

	peerBytes := make([]byte, 0, len(allPeers)*6)

	for addr := range allPeers {
		parts := strings.Split(addr, ":")
		if len(parts) != 2 {
			continue
		}

		ipParts := strings.Split(parts[0], ".")
		if len(ipParts) != 4 {
			continue
		}

		port, err := strconv.ParseUint(parts[1], 10, 16)
		if err != nil {
			continue
		}

		peerBytes = append(peerBytes,
			byte(atoi(ipParts[0])),
			byte(atoi(ipParts[1])),
			byte(atoi(ipParts[2])),
			byte(atoi(ipParts[3])),
			byte(port>>8),
			byte(port&0xFF),
		)
	}

	return &TrackerResponse{
		Peers:    string(peerBytes),
		Interval: finalInterval,
	}, nil
}

// --------------------------------------------------------------------------------------------- //

/*
atoi converts a string to an integer, ignoring errors.
It is a helper function for parsing IP addresses.

Parameters:
  - s: String to convert to an integer.

Returns:
  - int: The parsed integer, or 0 if parsing fails.
*/
func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// --------------------------------------------------------------------------------------------- //
