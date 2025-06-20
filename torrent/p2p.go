package torrent

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

// --------------------------------------------------------------------------------------------- //

type Handshake struct {
	ProtocolNameLength byte
	Protocol           [19]byte
	Reserved           [8]byte
	InfoHash           [20]byte
	PeerID             [20]byte
}

// --------------------------------------------------------------------------------------------- //

func (Torrent *TorrentFile) PerformHandshake(peer Peer) (string, error) {
	addr := fmt.Sprintf("%s:%d", peer.IP, peer.Port)
	myIP := "195.133.75.235" // TODO: get own external IP address
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

	fmt.Printf("Sending handshake to %s: ProtocolNameLength=%d, InfoHash=%x, PeerID=%s\n",
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

	fmt.Printf("Received handshake from %s: ProtocolNameLength=%d, Protocol=%s, InfoHash=%x, PeerID=%s\n",
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
				fmt.Printf("Peer %s:%d: CTP goroutine completed\n", peer.IP, peer.Port)
			}()

			remotePeerID, err := Torrent.PerformHandshake(p)
			if err != nil {
				return
			}

			fmt.Printf("Peer %s:%d handshake successful, remotePeerID: %s\n",
				p.IP, p.Port, remotePeerID)
		}(peer)
	}

	wg.Wait()
	fmt.Printf("Connected to %d peers\n", len(Torrent.Peers))
}

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

type Message struct {
	ID      MessageID
	Payload []byte
}

// --------------------------------------------------------------------------------------------- //

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
			fmt.Printf("Peer %s:%d: sent message ID=%d, payload length=%d\n", peer.IP, peer.Port, msg.ID, len(msg.Payload))
			return nil
		}

		fmt.Printf("Peer %s:%d: attempt %d failed to send message ID = %d: %v\n", peer.IP, peer.Port, attempt, msg.ID, err)
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("Failed to send message to %s:%d after 3 attempts", peer.IP, peer.Port)
}

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
		fmt.Printf("Peer %s:%d: received keep-alive\n", peer.IP, peer.Port)
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

	fmt.Printf("Peer %s:%d: received message ID=%d, payload length=%d\n", peer.IP, peer.Port, msg.ID, len(msg.Payload))

	return msg, nil
}

// --------------------------------------------------------------------------------------------- //

type PieceResult struct {
	Index int
	Data  []byte
}

// --------------------------------------------------------------------------------------------- //

func (Torrent *TorrentFile) DownloadFromPeer(peer *Peer, pieceChan chan<- PieceResult, wg *sync.WaitGroup) {
	defer func() {
		if peer.Connection != nil {
			peer.Connection.Close()
		}

		wg.Done()
		fmt.Printf("Peer %s:%d: DownloadFromPeer completed\n", peer.IP, peer.Port)
	}()

	fmt.Printf("Peer %s:%d: Starting download\n", peer.IP, peer.Port)

	for attempt := 1; attempt <= 3; attempt++ {
		err := Torrent.SendMessage(peer, Message{ID: Interested})
		if err == nil {
			break
		}

		fmt.Printf("Peer %s:%d: attempt %d failed to send Interested: %v\n", peer.IP, peer.Port, attempt, err)

		if attempt == 3 {
			return
		}

		time.Sleep(2 * time.Second)
	}

	for {
		msg, err := Torrent.ReceiveMessage(peer)
		if err != nil {
			fmt.Printf("Peer %s:%d: failed to receive message: %v\n", peer.IP, peer.Port, err)
			return
		}

		if msg == nil {
			fmt.Printf("Peer %s:%d: received keep-alive\n", peer.IP, peer.Port)
			continue
		}

		switch msg.ID {
		case Bitfield:
			peer.Bitfield = msg.Payload
			fmt.Printf("Peer %s:%d: received Bitfield (length=%d)\n", peer.IP, peer.Port, len(peer.Bitfield))

		case Unchoke:
			peer.Choked = false
			fmt.Printf("Peer %s:%d: unchoked\n", peer.IP, peer.Port)

		case Choke:
			peer.Choked = true
			fmt.Printf("Peer %s:%d: choked\n", peer.IP, peer.Port)
		}

		if !peer.Choked && peer.Bitfield != nil {
			fmt.Printf("Peer %s:%d: ready to download pieces\n", peer.IP, peer.Port)
			break
		}
	}

	blockSize := 1 << 14 // 16 KB // TODO: to constant

	for {
		if peer.Choked {
			fmt.Printf("Peer %s:%d: choked, waiting for Unchoke\n", peer.IP, peer.Port)

			for {
				msg, err := Torrent.ReceiveMessage(peer)
				if err != nil {
					fmt.Printf("Peer %s:%d: failed to receive message while choked: %v\n", peer.IP, peer.Port, err)
					return
				}

				if msg == nil {
					continue
				}

				switch msg.ID {
				case Unchoke:
					peer.Choked = false
					fmt.Printf("Peer %s:%d: unchoked\n", peer.IP, peer.Port)

				case Choke:
					peer.Choked = true
					fmt.Printf("Peer %s:%d: choked\n", peer.IP, peer.Port)
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
			fmt.Printf("Peer %s:%d: no more pieces to download\n", peer.IP, peer.Port)
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
				fmt.Printf("Peer %s:%d: failed to send Request for piece %d, offset %d: %v\n",
					peer.IP, peer.Port, pieceIndex, offset, err)
				Torrent.DownloadMutex.Lock()
				Torrent.Downloaded[pieceIndex] = false
				Torrent.DownloadMutex.Unlock()

				return
			}

			for {
				msg, err := Torrent.ReceiveMessage(peer)
				if err != nil {
					fmt.Printf("Peer %s:%d: failed to receive Piece for piece %d, offset %d: %v\n",
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
						fmt.Printf("Peer %s:%d: invalid Piece payload length %d for piece %d, offset %d\n",
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
					fmt.Printf("Peer %s:%d: choked during piece %d, offset %d\n",
						peer.IP, peer.Port, pieceIndex, offset)

					Torrent.DownloadMutex.Lock()
					Torrent.Downloaded[pieceIndex] = false
					Torrent.DownloadMutex.Unlock()

					continue

				default:
					fmt.Printf("Peer %s:%d: unexpected message ID %d for piece %d, offset %d\n",
						peer.IP, peer.Port, msg.ID, pieceIndex, offset)
					continue
				}

				break
			}
		}

		hash := sha1.Sum(data)

		if !bytes.Equal(hash[:], Torrent.PieceHashes[pieceIndex][:]) {
			fmt.Printf("Peer %s:%d: piece %d hash mismatch\n", peer.IP, peer.Port, pieceIndex)

			Torrent.DownloadMutex.Lock()
			Torrent.Downloaded[pieceIndex] = false
			Torrent.DownloadMutex.Unlock()

			continue
		}

		fmt.Printf("Peer %s:%d: downloaded piece %d (length=%d)\n",
			peer.IP, peer.Port, pieceIndex, len(data))

		pieceChan <- PieceResult{
			Index: pieceIndex,
			Data:  data,
		}
	}
}

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

func (Torrent *TorrentFile) StartDownload(outputPath string) error {
	err := Torrent.InitializePieces()
	if err != nil {
		return fmt.Errorf("failed to initialize pieces: %v", err)
	}

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
			fmt.Printf("Peer %s:%d: invalid connection, skipping\n", peer.IP, peer.Port)
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(pp *Peer) {
			defer func() {
				<-sem
				fmt.Printf("Peer %s:%d: StartDownload goroutine completed\n", pp.IP, pp.Port)
			}()

			Torrent.DownloadFromPeer(pp, pieceChan, &wg)
		}(peer)
	}

	go func() {
		wg.Wait()
		close(pieceChan)
		fmt.Println("All download goroutines completed, pieceChan closed")
	}()

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer file.Close()

	completed := make(map[int]bool)

	for piece := range pieceChan {
		Torrent.DownloadMutex.Lock()
		if completed[piece.Index] {
			fmt.Printf("Piece %d already written, skipping\n", piece.Index)
			Torrent.DownloadMutex.Unlock()

			continue
		}

		_, err := file.Seek(int64(piece.Index)*Torrent.PieceLength, 0)
		if err != nil {
			fmt.Printf("Failed to seek for piece %d: %v\n", piece.Index, err)
			Torrent.Downloaded[piece.Index] = false
			Torrent.DownloadMutex.Unlock()

			continue
		}

		_, err = file.Write(piece.Data)
		if err != nil {
			fmt.Printf("Failed to write piece %d: %v\n", piece.Index, err)
			Torrent.Downloaded[piece.Index] = false
			Torrent.DownloadMutex.Unlock()

			continue
		}

		completed[piece.Index] = true
		Torrent.DownloadMutex.Unlock()

		fmt.Printf("Wrote piece %d to %s, Progress: %.2f%% (%d/%d pieces)\n",
			piece.Index, outputPath, float64(len(completed))/float64(Torrent.NumPieces)*100, len(completed), Torrent.NumPieces)
	}

	if len(completed) != Torrent.NumPieces {
		return fmt.Errorf("Download incomplete: %d/%d pieces written", len(completed), Torrent.NumPieces)
	}

	return nil
}

func (Torrent *TorrentFile) RefreshPeer() {
	go func() {
		for {
			resp, err := Torrent.SendTrackerResponse()
			if err != nil {
				fmt.Printf("Failed to refresh peers: %v\n", err)
				time.Sleep(60 * time.Second)

				continue
			}

			newPeers, err := Torrent.ParsePeers(resp.Peers)
			if err != nil {
				fmt.Printf("Failed to parse new peers: %v\n", err)
				time.Sleep(60 * time.Second)

				continue
			}

			Torrent.ConnectToPeers(newPeers)
			time.Sleep(time.Duration(resp.Interval) * time.Second)
		}
	}()
}

// --------------------------------------------------------------------------------------------- //
