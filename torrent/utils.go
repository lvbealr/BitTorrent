package torrent

import (
	crand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
)

// --------------------------------------------------------------------------------------------- //

/*
ParsePeers converts a compact peer list from a tracker response into a slice of Peer structs.
The peer list is a binary string where each peer is represented by 6 bytes (4 for IP, 2 for port).

Parameters:
  - Torrent: Pointer to the TorrentFile (implicitly used for method context).
  - peers: String containing the compact peer list.

Returns:
  - []Peer: Slice of Peer structs with IP and port information.
  - error: Non-nil if the peer list length is invalid (not a multiple of 6).
*/
func (Torrent *TorrentFile) ParsePeers(peers string) ([]Peer, error) {
	peerBytes := []byte(peers)
	if len(peerBytes)%6 != 0 {
		return nil, fmt.Errorf("Invalid peers length: %d (must be multiple of 6)\n", len(peerBytes))
	}

	var result []Peer

	for i := 0; i < len(peerBytes); i += 6 {
		ip := fmt.Sprintf("%d.%d.%d.%d", peerBytes[i], peerBytes[i+1], peerBytes[i+2], peerBytes[i+3])
		port := binary.BigEndian.Uint16(peerBytes[i+4 : i+6])
		result = append(result, Peer{IP: ip, Port: port})
	}

	return result, nil
}

// --------------------------------------------------------------------------------------------- //

/*
GetInfoHash retrieves the SHA-1 hash of the torrent's info dictionary.
It returns the precomputed InfoHash stored in the TorrentFile.

Parameters:
  - Torrent: Pointer to the TorrentFile containing the InfoHash.

Returns:
  - [20]byte: The 20-byte SHA-1 hash of the info dictionary.
  - error: Always nil (included for interface compatibility).
*/
func (Torrent *TorrentFile) GetInfoHash() ([20]byte, error) {
	return Torrent.Info.InfoHash, nil
}

// --------------------------------------------------------------------------------------------- //

/*
GeneratePeerID creates a unique peer ID for the client.
It combines a fixed prefix with random characters to form a 20-byte ID.

Parameters:
  - Torrent: Pointer to the TorrentFile (implicitly used for method context).

Returns:
  - string: A 20-character peer ID starting with "-GT0001-".
  - error: Non-nil if random byte generation fails.
*/
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

// --------------------------------------------------------------------------------------------- //

/*
GetTotalSize calculates the total size of the torrent's content.
For single-file torrents, it returns the file length; for multi-file torrents, it sums the file lengths.

Parameters:
  - Torrent: Pointer to the TorrentFile containing file metadata.

Returns:
  - uint64: Total size of the torrent content in bytes.
  - error: Always nil (included for interface compatibility).
*/
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

// --------------------------------------------------------------------------------------------- //

/*
isHTTP checks if a URL uses the HTTP or HTTPS protocol.
It is used to identify HTTP-based tracker URLs.

Parameters:
  - url: The URL string to check.

Returns:
  - bool: True if the URL starts with "http://" or "https://", false otherwise.
*/
func isHTTP(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

// --------------------------------------------------------------------------------------------- //

/*
isUDP checks if a URL uses the UDP protocol.
It is used to identify UDP-based tracker URLs.

Parameters:
  - url: The URL string to check.

Returns:
  - bool: True if the URL starts with "udp://", false otherwise.
*/
func isUDP(url string) bool {
	return strings.HasPrefix(url, "udp://")
}

// --------------------------------------------------------------------------------------------- //

/*
GenerateTransactionID creates a random 32-bit transaction ID for tracker requests.
It uses cryptographically secure random bytes to ensure uniqueness.

Parameters:
  - Torrent: Pointer to the TorrentFile (implicitly used for method context).

Returns:
  - uint32: A random 32-bit transaction ID.
  - error: Non-nil if random byte generation fails.
*/
func (Torrent *TorrentFile) GenerateTransactionID() (uint32, error) {
	var buf [4]byte

	_, err := crand.Read(buf[:])
	if err != nil {
		return 0, fmt.Errorf("Generating transaction ID error: %v\n", err)
	}

	return binary.BigEndian.Uint32(buf[:]), nil
}

// --------------------------------------------------------------------------------------------- //

/*
BuildFileInfo constructs the FileInfo slice for the torrent's files.
It creates file paths and offsets for single-file or multi-file torrents.

Parameters:
  - Torrent: Pointer to the TorrentFile containing file metadata.
  - outputDir: Directory where the files will be saved.

Returns:
  - error: Always nil (no error conditions are currently checked).
*/
func (Torrent *TorrentFile) BuildFileInfo(outputDir string) error {
	Torrent.Files = nil

	if len(Torrent.Info.Files) == 0 {
		Torrent.Files = append(Torrent.Files, FileInfo{
			Path:   filepath.Join(outputDir, Torrent.Info.Name),
			Length: Torrent.Info.Length,
			Offset: 0,
		})
	} else {
		baseDir := filepath.Join(outputDir, Torrent.Info.Name)
		var offset int64 = 0

		for _, fileEntry := range Torrent.Info.Files {
			parts := []string{baseDir}
			parts = append(parts, fileEntry.Path...)
			fullPath := filepath.Join(parts...)

			Torrent.Files = append(Torrent.Files, FileInfo{
				Path:   fullPath,
				Length: fileEntry.Length,
				Offset: offset,
			})

			offset += fileEntry.Length
		}
	}

	return nil
}

// --------------------------------------------------------------------------------------------- //

/*
GetExternalIP retrieves the client's external IP address.
It queries an external service (httpbin.org) to obtain the public IP.

Parameters:
  - None: No parameters are required.

Returns:
  - string: The external IP address as a string.
  - error: Non-nil if the HTTP request, response reading, or JSON parsing fails.
*/
func GetExternalIP() (string, error) {
	resp, err := http.Get("http://httpbin.org/ip")
	if err != nil {
		return "", fmt.Errorf("[ERROR]\tFailed to get external IP: %v\n", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("[ERROR]\tFailed to read response body: %v\n", err)
	}

	var result struct {
		Origin string `json:"origin"`
	}

	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", fmt.Errorf("[ERROR]\tFailed to parse JSON: %v\n", err)
	}

	return result.Origin, nil
}

// --------------------------------------------------------------------------------------------- //
