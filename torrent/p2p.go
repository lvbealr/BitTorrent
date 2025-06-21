package torrent

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// --------------------------------------------------------------------------------------------- //

/*
Handshake represents the structure of a BitTorrent protocol handshake message.
It is used to initiate a connection with a peer and verify compatibility.

Fields:
  - ProtocolNameLength: Length of the protocol name (typically 19 for "BitTorrent protocol").
  - Protocol: Fixed-size array containing the protocol name.
  - Reserved: Reserved bytes for protocol extensions.
  - InfoHash: 20-byte SHA-1 hash of the torrent's info dictionary.
  - PeerID: 20-byte unique identifier for the peer.
*/
type Handshake struct {
	ProtocolNameLength byte
	Protocol           [19]byte
	Reserved           [8]byte
	InfoHash           [20]byte
	PeerID             [20]byte
}

// --------------------------------------------------------------------------------------------- //

/*
PerformHandshake executes the BitTorrent handshake with a specified peer.
It establishes a TCP connection, sends a handshake message, and verifies the response.

Parameters:
  - Torrent: Pointer to the TorrentFile containing metadata like InfoHash.
  - peer: Peer struct containing the IP and port of the peer to connect to.

Returns:
  - string: Remote peer's PeerID if the handshake is successful.
  - error: Non-nil if connection, handshake sending, or response validation fails.
*/
func (Torrent *TorrentFile) PerformHandshake(peer Peer) (string, error) {
	addr := fmt.Sprintf("%s:%d", peer.IP, peer.Port)
	myIP, err := GetExternalIP()
	if err != nil {
		return "", err
	}

	if peer.IP == myIP {
		return "", fmt.Errorf("Skip handshake with self: %s", addr)
	}

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("Connecting to peer failed: %v", err)
	}

	protocol := "BitTorrent protocol"

	var hs Handshake
	hs.ProtocolNameLength = byte(len(protocol))
	copy(hs.Protocol[:], protocol)
	hs.InfoHash = Torrent.Info.InfoHash

	peerID, err := Torrent.GeneratePeerID()
	if err != nil {
		conn.Close()
		return "", err
	}

	copy(hs.PeerID[:], peerID)

	log.Printf("[INFO]\tSending handshake to %s: ProtocolNameLength=%d, InfoHash=%x, PeerID=%s\n",
		addr, hs.ProtocolNameLength, hs.InfoHash, peerID)
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))

	err = binary.Write(conn, binary.BigEndian, &hs)
	if err != nil {
		conn.Close()
		return "", fmt.Errorf("Sending handshake error: %v\n", err)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var response Handshake

	err = binary.Read(conn, binary.BigEndian, &response)
	if err != nil {
		conn.Close()
		return "", fmt.Errorf("Reading handshake error: %v\n", err)
	}

	log.Printf("[INFO]\tReceived handshake from %s: ProtocolNameLength=%d, Protocol=%s, InfoHash=%x, PeerID=%s\n",
		addr, response.ProtocolNameLength, string(response.Protocol[:]), response.InfoHash, string(response.PeerID[:]))
	if response.ProtocolNameLength != 19 || string(response.Protocol[:]) != protocol {
		conn.Close()
		return "", fmt.Errorf("Invalid protocol in handshake\n")
	}

	if !bytes.Equal(response.InfoHash[:], Torrent.Info.InfoHash[:]) {
		conn.Close()
		return "", fmt.Errorf("Info hash mismatch in handshake\n")
	}

	remotePeerID := string(response.PeerID[:])

	Torrent.PeersMutex.Lock()
	Torrent.Peers = append(Torrent.Peers, Peer{
		IP:         peer.IP,
		Port:       peer.Port,
		PeerID:     remotePeerID,
		Connection: conn,
		Choked:     true,
		Bitfield:   nil,
	})
	Torrent.PeersMutex.Unlock()

	return remotePeerID, nil
}

// --------------------------------------------------------------------------------------------- //

/*
ConnectToPeers establishes connections with a list of peers by performing handshakes.
It uses goroutines to handle multiple peers concurrently, with a semaphore to limit connections.

Parameters:
  - Torrent: Pointer to the TorrentFile containing metadata.
  - peers: Slice of Peer structs to connect to.

Returns:
  - None: The function updates Torrent.Peers and logs connection status.
*/
func (Torrent *TorrentFile) ConnectToPeers(peers []Peer) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)

	for _, peer := range peers {
		wg.Add(1)
		sem <- struct{}{}

		go func(p Peer) {
			defer func() {
				<-sem
				wg.Done()
				log.Printf("[INFO]\tPeer %s:%d: CTP goroutine completed\n", peer.IP, peer.Port)
			}()

			remotePeerID, err := Torrent.PerformHandshake(p)
			if err != nil {
				return
			}

			log.Printf("[INFO]\tPeer %s:%d handshake successful, remotePeerID: %s\n",
				p.IP, p.Port, remotePeerID)
		}(peer)
	}

	wg.Wait()
	log.Printf("[INFO]\tConnected to %d peers\n", len(Torrent.Peers))
}

// --------------------------------------------------------------------------------------------- //

/*
InitializePieces sets up the piece-related metadata for the torrent.
It extracts piece length, number of pieces, and piece hashes from the torrent's info.

Parameters:
  - Torrent: Pointer to the TorrentFile to initialize.

Returns:
  - error: Non-nil if the pieces data is invalid.
*/
func (Torrent *TorrentFile) InitializePieces() error {
	Torrent.PieceLength = Torrent.Info.PieceLength
	pieces := Torrent.Info.Pieces
	if len(pieces)%20 != 0 {
		return fmt.Errorf("Invalid pieces length: %d\n", len(pieces))
	}

	Torrent.NumPieces = len(pieces) / 20
	Torrent.PieceHashes = make([][20]byte, Torrent.NumPieces)

	for i := 0; i < Torrent.NumPieces; i++ {
		copy(Torrent.PieceHashes[i][:], pieces[i*20:(i+1)*20])
	}

	Torrent.Downloaded = make([]bool, Torrent.NumPieces)

	return nil
}

// --------------------------------------------------------------------------------------------- //

/*
MessageID is an enumeration of BitTorrent protocol message types.
It defines the possible message IDs used in peer communication.

Values:
  - Choke: Indicates the peer is choked (not sending data).
  - Unchoke: Indicates the peer is unchoked (can send data).
  - Interested: Indicates interest in downloading from the peer.
  - NotInterested: Indicates no interest in downloading from the peer.
  - Have: Indicates the peer has a specific piece.
  - Bitfield: Indicates which pieces the peer has.
  - Request: Requests a block of a piece.
  - Piece: Delivers a block of a piece.
  - Cancel: Cancels a previous request.
*/
type MessageID uint8

const (
	Choke MessageID = iota
	Unchoke
	Interested
	NotInterested
	Have
	Bitfield
	Request
	Piece
	Cancel
)

// --------------------------------------------------------------------------------------------- //

/*
Message represents a BitTorrent protocol message.
It contains the message type and its associated payload.

Fields:
  - ID: The type of message (e.g., Choke, Piece).
  - Payload: The message's data, if any.
*/
type Message struct {
	ID      MessageID
	Payload []byte
}

// --------------------------------------------------------------------------------------------- //

/*
SendMessage sends a BitTorrent protocol message to a peer.
It serializes the message with its length prefix and retries up to three times.

Parameters:
  - Torrent: Pointer to the TorrentFile.
  - peer: Pointer to the Peer to send the message to.
  - msg: The Message to send.

Returns:
  - error: Non-nil if the connection is invalid or all send attempts fail.
*/
func (Torrent *TorrentFile) SendMessage(peer *Peer, msg Message) error {
	if peer.Connection == nil {
		return fmt.Errorf("No connection to peer %s:%d", peer.IP, peer.Port)
	}

	var buf bytes.Buffer
	length := uint32(len(msg.Payload) + 1)
	binary.Write(&buf, binary.BigEndian, length)
	binary.Write(&buf, binary.BigEndian, msg.ID)

	if len(msg.Payload) > 0 {
		buf.Write(msg.Payload)
	}

	for attempt := 1; attempt <= 3; attempt++ {
		peer.Connection.SetWriteDeadline(time.Now().Add(60 * time.Second))
		_, err := peer.Connection.Write(buf.Bytes())
		if err == nil {
			log.Printf("[INFO]\tPeer %s:%d: sent message ID=%d, payload length=%d\n", peer.IP, peer.Port, msg.ID, len(msg.Payload))
			return nil
		}

		log.Printf("[FAIL]\tPeer %s:%d: attempt %d failed to send message ID = %d: %v\n", peer.IP, peer.Port, attempt, msg.ID, err)
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("Failed to send message to %s:%d after 3 attempts", peer.IP, peer.Port)
}

// --------------------------------------------------------------------------------------------- //

/*
ReceiveMessage reads and parses a BitTorrent protocol message from a peer.
It handles keep-alive messages (zero length) and validates message size.

Parameters:
  - Torrent: Pointer to the TorrentFile.
  - peer: Pointer to the Peer to receive the message from.

Returns:
  - *Message: Pointer to the received message, or an empty message for keep-alive.
  - error: Non-nil if the connection is invalid, message is too large, or read fails.
*/
func (Torrent *TorrentFile) ReceiveMessage(peer *Peer) (*Message, error) {
	if peer.Connection == nil {
		return nil, fmt.Errorf("No connection to peer %s:%d", peer.IP, peer.Port)
	}

	peer.Connection.SetReadDeadline(time.Now().Add(60 * time.Second))
	var length uint32
	err := binary.Read(peer.Connection, binary.BigEndian, &length)
	if err != nil {
		return nil, fmt.Errorf("Reading message length from %s:%d: %v", peer.IP, peer.Port, err)
	}

	if length == 0 {
		log.Printf("[INFO]\tPeer %s:%d: received keep-alive\n", peer.IP, peer.Port)
		return &Message{}, nil
	}

	if length > 1<<20 {
		return nil, fmt.Errorf("Message too large: %d bytes from %s:%d", length, peer.IP, peer.Port)
	}

	buf := make([]byte, length)
	_, err = io.ReadFull(peer.Connection, buf)
	if err != nil {
		return nil, fmt.Errorf("Reading message from %s:%d: %v", peer.IP, peer.Port, err)
	}

	msg := &Message{
		ID:      MessageID(buf[0]),
		Payload: buf[1:],
	}

	log.Printf("[INFO]\tPeer %s:%d: received message ID=%d, payload length=%d\n", peer.IP, peer.Port, msg.ID, len(msg.Payload))

	return msg, nil
}

// --------------------------------------------------------------------------------------------- //

/*
PieceResult represents a downloaded piece of the torrent.
It contains the piece index and its data.

Fields:
  - Index: The index of the downloaded piece.
  - Data: The byte slice containing the piece's data.
*/
type PieceResult struct {
	Index int
	Data  []byte
}

// --------------------------------------------------------------------------------------------- //

/*
DownloadFromPeer downloads pieces from a specific peer.
It sends an Interested message, processes incoming messages, and requests pieces.

Parameters:
  - Torrent: Pointer to the TorrentFile containing piece metadata.
  - peer: Pointer to the Peer to download from.
  - pieceChan: Channel to send downloaded pieces to.
  - wg: WaitGroup to signal completion.

Returns:
  - None: The function sends PieceResult to pieceChan and logs status.
*/
func (Torrent *TorrentFile) DownloadFromPeer(peer *Peer, pieceChan chan<- PieceResult, wg *sync.WaitGroup) {
	defer func() {
		if peer.Connection != nil {
			peer.Connection.Close()
		}

		wg.Done()
		log.Printf("[INFO]\tPeer %s:%d: DownloadFromPeer completed\n", peer.IP, peer.Port)
	}()

	log.Printf("[INFO]\tPeer %s:%d: Starting download\n", peer.IP, peer.Port)

	for attempt := 1; attempt <= 3; attempt++ {
		err := Torrent.SendMessage(peer, Message{ID: Interested})
		if err == nil {
			break
		}

		log.Printf("[FAIL]\tPeer %s:%d: attempt %d failed to send Interested: %v\n", peer.IP, peer.Port, attempt, err)

		if attempt == 3 {
			return
		}

		time.Sleep(2 * time.Second)
	}

	for {
		msg, err := Torrent.ReceiveMessage(peer)
		if err != nil {
			log.Printf("[FAIL]\tPeer %s:%d: failed to receive message: %v\n", peer.IP, peer.Port, err)
			return
		}

		if msg == nil {
			log.Printf("[INFO]\tPeer %s:%d: received keep-alive\n", peer.IP, peer.Port)
			continue
		}

		switch msg.ID {
		case Bitfield:
			peer.Bitfield = msg.Payload
			log.Printf("[INFO]\tPeer %s:%d: received Bitfield (length=%d)\n", peer.IP, peer.Port, len(peer.Bitfield))

		case Unchoke:
			peer.Choked = false
			log.Printf("[INFO]\tPeer %s:%d: unchoked\n", peer.IP, peer.Port)

		case Choke:
			peer.Choked = true
			log.Printf("[INFO]\tPeer %s:%d: choked\n", peer.IP, peer.Port)
		}

		if !peer.Choked && peer.Bitfield != nil {
			log.Printf("[INFO]\tPeer %s:%d: ready to download pieces\n", peer.IP, peer.Port)
			break
		}
	}

	const (
		blockSize = 1 << 14 // 16 kB
	)

	for {
		if peer.Choked {
			log.Printf("[INFO]\tPeer %s:%d: choked, waiting for Unchoke\n", peer.IP, peer.Port)

			for {
				msg, err := Torrent.ReceiveMessage(peer)
				if err != nil {
					log.Printf("[FAIL]\tPeer %s:%d: failed to receive message while choked: %v\n", peer.IP, peer.Port, err)
					return
				}

				if msg == nil {
					continue
				}

				switch msg.ID {
				case Unchoke:
					peer.Choked = false
					log.Printf("[INFO]\tPeer %s:%d: unchoked\n", peer.IP, peer.Port)

				case Choke:
					peer.Choked = true
					log.Printf("[INFO]\tPeer %s:%d: choked\n", peer.IP, peer.Port)
				}

				if !peer.Choked {
					break
				}
			}
		}

		Torrent.DownloadMutex.Lock()
		pieceIndex := -1

		for i, downloaded := range Torrent.Downloaded {
			if !downloaded && Torrent.HasPiece(peer.Bitfield, i) {
				pieceIndex = i
				Torrent.Downloaded[i] = true

				break
			}
		}

		Torrent.DownloadMutex.Unlock()

		if pieceIndex == -1 {
			log.Printf("[INFO]\tPeer %s:%d: no more pieces to download\n", peer.IP, peer.Port)
			return
		}

		pieceLength := Torrent.PieceLength

		if pieceIndex == Torrent.NumPieces-1 {
			pieceLength = Torrent.Info.Length % Torrent.PieceLength

			if pieceLength == 0 {
				pieceLength = Torrent.PieceLength
			}
		}

		data := make([]byte, 0, pieceLength)

		for offset := int64(0); offset < pieceLength; offset += int64(blockSize) {
			remaining := pieceLength - offset

			if remaining > int64(blockSize) {
				remaining = int64(blockSize)
			}

			payload := new(bytes.Buffer)
			binary.Write(payload, binary.BigEndian, uint32(pieceIndex))
			binary.Write(payload, binary.BigEndian, uint32(offset))
			binary.Write(payload, binary.BigEndian, uint32(remaining))

			err := Torrent.SendMessage(peer, Message{ID: Request, Payload: payload.Bytes()})
			if err != nil {
				log.Printf("[FAIL]\tPeer %s:%d: failed to send Request for piece %d, offset %d: %v\n",
					peer.IP, peer.Port, pieceIndex, offset, err)
				Torrent.DownloadMutex.Lock()
				Torrent.Downloaded[pieceIndex] = false
				Torrent.DownloadMutex.Unlock()

				return
			}

			for {
				msg, err := Torrent.ReceiveMessage(peer)
				if err != nil {
					log.Printf("[FAIL]\tPeer %s:%d: failed to receive Piece for piece %d, offset %d: %v\n",
						peer.IP, peer.Port, pieceIndex, offset, err)
					Torrent.DownloadMutex.Lock()
					Torrent.Downloaded[pieceIndex] = false
					Torrent.DownloadMutex.Unlock()

					return
				}

				if msg == nil {
					continue
				}

				switch msg.ID {
				case Piece:
					if len(msg.Payload) < 8 {
						log.Printf("[ERROR]\tPeer %s:%d: invalid Piece payload length %d for piece %d, offset %d\n",
							peer.IP, peer.Port, len(msg.Payload), pieceIndex, offset)
						Torrent.DownloadMutex.Lock()
						Torrent.Downloaded[pieceIndex] = false
						Torrent.DownloadMutex.Unlock()

						return
					}

					data = append(data, msg.Payload[8:]...)
					break

				case Choke:
					peer.Choked = true
					log.Printf("[ERROR]\tPeer %s:%d: choked during piece %d, offset %d\n",
						peer.IP, peer.Port, pieceIndex, offset)

					Torrent.DownloadMutex.Lock()
					Torrent.Downloaded[pieceIndex] = false
					Torrent.DownloadMutex.Unlock()

					continue

				default:
					log.Printf("[ERROR]\tPeer %s:%d: unexpected message ID %d for piece %d, offset %d\n",
						peer.IP, peer.Port, msg.ID, pieceIndex, offset)
					continue
				}

				break
			}
		}

		hash := sha1.Sum(data)

		if !bytes.Equal(hash[:], Torrent.PieceHashes[pieceIndex][:]) {
			log.Printf("[ERROR]\tPeer %s:%d: piece %d hash mismatch\n", peer.IP, peer.Port, pieceIndex)

			Torrent.DownloadMutex.Lock()
			Torrent.Downloaded[pieceIndex] = false
			Torrent.DownloadMutex.Unlock()

			continue
		}

		log.Printf("[INFO]\tPeer %s:%d: downloaded piece %d (length=%d)\n",
			peer.IP, peer.Port, pieceIndex, len(data))

		pieceChan <- PieceResult{
			Index: pieceIndex,
			Data:  data,
		}
	}
}

// --------------------------------------------------------------------------------------------- //

/*
HasPiece checks if a peer has a specific piece based on its bitfield.
The bitfield is a byte slice where each bit represents a piece's availability.

Parameters:
  - Torrent: Pointer to the TorrentFile.
  - bitfield: Byte slice representing the peer's available pieces.
  - index: Index of the piece to check.

Returns:
  - bool: True if the peer has the piece, false otherwise.
*/
func (Torrent *TorrentFile) HasPiece(bitfield []byte, index int) bool {
	if bitfield == nil {
		return false
	}

	byteIndex := index / 8
	bitIndex := index % 8

	if byteIndex >= len(bitfield) {
		return false
	}

	return (bitfield[byteIndex]>>(7-bitIndex))&1 == 1
}

// --------------------------------------------------------------------------------------------- //

/*
StartDownload initiates the download process for the torrent.
It initializes pieces, creates output files, and spawns goroutines to download from peers.

Parameters:
  - Torrent: Pointer to the TorrentFile containing metadata and peer connections.
  - outputDir: Directory where downloaded files will be saved.

Returns:
  - error: Non-nil if piece initialization, file creation, or download fails.
*/
func (Torrent *TorrentFile) StartDownload(outputDir string) error {
	err := Torrent.InitializePieces()
	if err != nil {
		return fmt.Errorf("Failed to initialize pieces: %v", err)
	}

	err = Torrent.BuildFileInfo(outputDir)
	if err != nil {
		return err
	}

	for i := range Torrent.Files {
		file := &Torrent.Files[i]
		dir := filepath.Dir(file.Path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("Failed to create directory %s: %v\n", dir, err)
		}

		f, err := os.OpenFile(file.Path, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			return fmt.Errorf("Failed to create file %s: %v\n", file.Path, err)
		}

		if err := f.Truncate(file.Length); err != nil {
			f.Close()
			return fmt.Errorf("Failed to truncate file %s: %v\n", file.Path, err)
		}

		file.Handle = f
	}

	defer func() {
		for _, file := range Torrent.Files {
			if file.Handle != nil {
				file.Handle.Close()
			}
		}
	}()

	pieceChan := make(chan PieceResult, Torrent.NumPieces)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)

	Torrent.PeersMutex.Lock()
	peers := make([]Peer, len(Torrent.Peers))
	copy(peers, Torrent.Peers)
	Torrent.PeersMutex.Unlock()

	for i := range peers {
		peer := &peers[i]

		if peer.Connection == nil {
			log.Printf("[FAIL]\tPeer %s:%d: invalid connection, skipping\n", peer.IP, peer.Port)
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(pp *Peer) {
			defer func() {
				<-sem
				log.Printf("[INFO]\tPeer %s:%d: StartDownload goroutine completed\n", pp.IP, pp.Port)
			}()

			Torrent.DownloadFromPeer(pp, pieceChan, &wg)
		}(peer)
	}

	go func() {
		wg.Wait()
		close(pieceChan)
		log.Printf("[INFO]\tAll download goroutines completed, pieceChan closed")
	}()

	completed := make(map[int]bool)
	barWidth := 50
	completedCount := 0

	var totalBytesLoaded int64
	type speedSample struct {
		bytes int64
		time  time.Time
	}

	speedSamples := make([]speedSample, 0)
	windowDuration := 5 * time.Second

	for piece := range pieceChan {
		Torrent.DownloadMutex.Lock()

		if completed[piece.Index] {
			log.Printf("[INFO]\tPiece %d already written, skipping\n", piece.Index)
			Torrent.DownloadMutex.Unlock()

			continue
		}

		pieceStart := int64(piece.Index) * Torrent.PieceLength
		pieceEnd := pieceStart + int64(len(piece.Data))

		for _, file := range Torrent.Files {
			fileStart := file.Offset
			fileEnd := file.Offset + file.Length

			start := max(pieceStart, fileStart)
			end := min(pieceEnd, fileEnd)

			if start >= end {
				continue
			}

			startInPiece := start - pieceStart
			endInPiece := end - pieceStart

			chunk := piece.Data[startInPiece:endInPiece]

			_, err := file.Handle.WriteAt(chunk, start-file.Offset)
			if err != nil {
				log.Printf("[ERROR]\tFailed writing to %s: %v", file.Path, err)
				Torrent.Downloaded[piece.Index] = false
			}
		}

		completed[piece.Index] = true
		completedCount++
		totalBytesLoaded += int64(len(piece.Data))
		Torrent.DownloadMutex.Unlock()

		now := time.Now()
		speedSamples = append(speedSamples, speedSample{bytes: int64(len(piece.Data)), time: now})

		cutoff := now.Add(-windowDuration)
		for len(speedSamples) > 0 && speedSamples[0].time.Before(cutoff) {
			speedSamples = speedSamples[1:]
		}

		var bytesInWindow int64
		for _, sample := range speedSamples {
			bytesInWindow += sample.bytes
		}

		windowSeconds := windowDuration.Seconds()
		if len(speedSamples) > 1 {
			windowSeconds = speedSamples[len(speedSamples)-1].time.Sub(speedSamples[0].time).Seconds()
		}

		speedMBps := 0.0
		if windowSeconds > 0 {
			speedMBps = float64(bytesInWindow) / windowSeconds / (1024 * 1024)
		}

		progress := float64(completedCount) / float64(Torrent.NumPieces)
		filled := int(progress * float64(barWidth))
		bar := strings.Repeat("»", filled) + strings.Repeat("-", barWidth-filled)
		percentage := progress * 100.0
		fmt.Printf("\r[%s]\t[%s] (%.2f/100%%) [%.2f MB/s]", Torrent.Info.Name, bar, percentage, speedMBps)
	}

	fmt.Println("\nDownload completed!")

	if len(completed) != Torrent.NumPieces {
		return fmt.Errorf("Download incomplete: %d/%d pieces written", len(completed), Torrent.NumPieces)
	}

	return nil
}

// --------------------------------------------------------------------------------------------- //

/*
RefreshPeer periodically refreshes the peer list by contacting trackers.
It runs in a goroutine, updating peers at intervals specified by the tracker.

Parameters:
  - Torrent: Pointer to the TorrentFile to refresh peers for.

Returns:
  - None: The function runs indefinitely, updating Torrent.Peers and logging status.
*/
func (Torrent *TorrentFile) RefreshPeer() {
	go func() {
		for {
			resp, err := Torrent.SendTrackerResponse()
			if err != nil {
				log.Printf("[FAIL]\tFailed to refresh peers: %v\n", err)
				time.Sleep(60 * time.Second)

				continue
			}

			newPeers, err := Torrent.ParsePeers(resp.Peers)
			if err != nil {
				log.Printf("[FAIL]\tFailed to parse new peers: %v\n", err)
				time.Sleep(60 * time.Second)

				continue
			}

			Torrent.ConnectToPeers(newPeers)
			time.Sleep(time.Duration(resp.Interval) * time.Second)
		}
	}()
}

// --------------------------------------------------------------------------------------------- //
