package main

import (
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"unicode"
)

type Request struct {
	curPieceLen int
	offset      int
}

// Stack represents a stack that holds a slice of empty interfaces (to allow for different types)
type Stack struct {
	elements []interface{}
}

// Push adds an element to the top of the stack
func (s *Stack) Push(element interface{}) {
	s.elements = append(s.elements, element)
}

// Pop removes and returns the top element of the stack. Returns an error if the stack is empty.
func (s *Stack) Pop() interface{} {
	topElement := s.elements[len(s.elements)-1]
	s.elements = s.elements[:len(s.elements)-1]
	return topElement
}

// Peek returns the top element of the stack without removing it. Returns an error if the stack is empty.
func (s *Stack) Peek() interface{} {
	return s.elements[len(s.elements)-1]
}

// IsEmpty checks if the stack is empty
func (s *Stack) IsEmpty() bool {
	return len(s.elements) == 0
}

// Size returns the number of elements in the stack
func (s *Stack) Size() int {
	return len(s.elements)
}

func reverse(slice *[]interface{}) {
	length := len(*slice)
	for i := 0; i < length/2; i++ {
		j := length - i - 1
		(*slice)[i], (*slice)[j] = (*slice)[j], (*slice)[i]
	}
}

func calculateSHA1(input []byte) string {
	sha1Hash := sha1.New()
	sha1Hash.Write(input)
	hashBytes := sha1Hash.Sum(nil)
	sha1String := fmt.Sprintf("%x", hashBytes)
	return sha1String
}

func getHexValue(input []byte) string {
	return fmt.Sprintf("%x", input)
}

var tracker string
var fileLength int
var pieceLength int
var pieceCount int
var piecesHash string
var infoHash string
var peers string
var peersArray []string

func decodeString(bencodedValue string) string {
	stack := &Stack{}
	i := 0
	for i < len(bencodedValue) {
		if bencodedValue[i] == 'l' || bencodedValue[i] == 'd' {
			stack.Push(bencodedValue[i])
			i = i + 1
		} else if bencodedValue[i] == 'e' {
			list := []interface{}{}
			for {
				if reflect.TypeOf(stack.Peek()).Kind() == reflect.Uint8 && stack.Peek().(uint8) == 'd' {
					benMap := make(map[string]interface{})
					for j := len(list) - 1; j >= 0; j -= 2 {
						if list[j].(string) == "announce" {
							tracker = list[j-1].(string)
						}
						if list[j].(string) == "length" {
							fileLength = list[j-1].(int)
						}
						if list[j].(string) == "piece length" {
							pieceLength = list[j-1].(int)
						}
						if list[j].(string) == "pieces" {
							for k := 0; k < len(list[j-1].(string)); k += 20 {
								pieceHash := getHexValue([]byte((list[j-1].(string))[k : k+20]))
								piecesHash += "\n" + pieceHash
								pieceCount++
							}
						}
						if list[j].(string) == "peers" {
							peersString := list[j-1].(string)
							for k := 0; k < len(peersString); k += 6 {
								peer := strconv.Itoa(int(peersString[k])) + "." + strconv.Itoa(int(peersString[k+1])) + "." + strconv.Itoa(int(peersString[k+2])) + "." + strconv.Itoa(int(peersString[k+3])) + ":" + strconv.Itoa(int((binary.BigEndian.Uint16)([]byte(peersString[k+4:k+6]))))
								peersArray = append(peersArray, peer)
								peers += peer + "\n"
							}
						}
						benMap[list[j].(string)] = list[j-1]
					}
					stack.Pop()
					stack.Push(benMap)
					break
				} else if reflect.TypeOf(stack.Peek()).Kind() == reflect.Uint8 && stack.Peek().(uint8) == 'l' {
					stack.Pop()
					reverse(&list)
					stack.Push(list)
					break
				} else {
					list = append(list, stack.Peek())
					stack.Pop()
				}
			}
			i = i + 1
		} else if unicode.IsDigit(rune(bencodedValue[i])) {
			var firstColonIndex int
			for j := i; j < len(bencodedValue); j++ {
				if bencodedValue[j] == ':' {
					firstColonIndex = j
					break
				}
			}
			lengthStr := bencodedValue[i:firstColonIndex]
			length, _ := strconv.Atoi(lengthStr)

			letter := bencodedValue[firstColonIndex+1 : firstColonIndex+1+length]
			stack.Push(letter)
			i = firstColonIndex + 1 + length
		} else if bencodedValue[i] == 'i' {
			lastIndex := 0
			for j := i + 1; j < len(bencodedValue); j++ {
				if bencodedValue[j] == 'e' {
					lastIndex = j
					break
				}
			}
			num, _ := strconv.Atoi(bencodedValue[i+1 : lastIndex])
			stack.Push(num)
			i = lastIndex + 1
		}
	}
	jsonOutput, _ := json.Marshal(stack.Peek())
	return string(jsonOutput)
}

func fillInfo(fileName string) {
	content, _ := os.ReadFile(fileName)
	bencodedValue := string(content)
	decodeString(bencodedValue)
	for i := 0; i < len(bencodedValue); i++ {
		if bencodedValue[i:i+4] == "info" {
			infoHash = calculateSHA1([]byte(bencodedValue[i+4 : len(bencodedValue)-1]))
			break
		}
	}
}

func makeRequest() {
	infoHashBytes, _ := hex.DecodeString(infoHash)
	// Query parameters
	params := url.Values{}
	params.Add("info_hash", string(infoHashBytes))
	params.Add("peer_id", "00112233445566778899")
	params.Add("port", "6881")
	params.Add("uploaded", "0")
	params.Add("downloaded", "0")
	params.Add("left", fmt.Sprint(fileLength))
	params.Add("compact", "1")

	// Construct the final URL with query parameters
	finalURL := fmt.Sprintf("%s?%s", tracker, params.Encode())

	// Making the GET request
	response, _ := http.Get(finalURL)
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	decodeString(string(body))
}

func getHandShakeMessage() []byte {
	handshakeMessage := []byte{19}
	handshakeMessage = append(handshakeMessage, []byte("BitTorrent protocol")...)
	handshakeMessage = append(handshakeMessage, []byte{0, 0, 0, 0, 0, 0, 0, 0}...)
	infoHashBytes, _ := hex.DecodeString(infoHash)
	handshakeMessage = append(handshakeMessage, infoHashBytes...)
	handshakeMessage = append(handshakeMessage, []byte("00112233445566778899")...)
	return handshakeMessage
}

func getConnection(peerId int) net.Conn {
	conn, _ := net.Dial("tcp", peersArray[peerId])
	conn.Write(getHandShakeMessage())
	buffer := make([]byte, 68)
	conn.Read(buffer)
	if buffer[0] == 0 {
		return getConnection(peerId + 1)
	}
	conn.Read(buffer)
	fmt.Printf("Peer %d is online...\n", peerId)
	return conn
}

func createAndSaveFile(pieceBytes []byte, filePath string) {
	file, _ := os.Create(filePath)
	defer file.Close()
	file.Write(pieceBytes)
}

func getPieceBytes(conn net.Conn, pieceID int) []byte {
	var message []byte
	// Interested
	message = append(message, 0, 0, 0, 1, 2)
	conn.Write(message)
	buffer := make([]byte, 5)
	// Unchoke
	for {
		conn.Read(buffer)
		if buffer[4] == 1 {
			break
		}
	}
	offset := 0
	var pieceBytes []byte

	var requestArray []Request

	if pieceID == pieceCount-1 {
		pieceLength = fileLength % pieceLength
	}

	tempPieceLength := pieceLength
	for pieceLength > 0 {
		var newRequest Request
		newRequest.curPieceLen = int(math.Min(math.Pow(2, 14), float64(pieceLength)))
		pieceLength -= newRequest.curPieceLen
		newRequest.offset = offset
		offset += int(math.Pow(2, 14))
		requestArray = append(requestArray, newRequest)
	}
	pieceLength = tempPieceLength
	numTasks := len(requestArray)
	for i := 0; i < numTasks; i++ {
		var request []byte
		// length
		request = append(request, 0, 0, 0, 13)
		// ID
		request = append(request, 6)
		// index
		request = append(request, make([]byte, 4)...)
		binary.BigEndian.PutUint32(request[len(request)-4:], uint32(pieceID))
		// offset
		request = append(request, make([]byte, 4)...)
		binary.BigEndian.PutUint32(request[len(request)-4:], uint32(requestArray[i].offset))
		// length
		request = append(request, make([]byte, 4)...)
		binary.BigEndian.PutUint32(request[len(request)-4:], uint32(requestArray[i].curPieceLen))
		// send request
		conn.Write(request)
		// read response
		var allcombined []byte
		total := 13 + requestArray[i].curPieceLen
		for len(allcombined) < total {
			tempBuffer := make([]byte, total-len(allcombined))
			bytesRead, _ := conn.Read(tempBuffer)
			allcombined = append(allcombined, tempBuffer[:bytesRead]...)
		}
		pieceBytes = append(pieceBytes, allcombined[13:]...)
	}
	return pieceBytes
}

func main() {

	command := os.Args[1]
	fileName := os.Args[2]
	serverAddress := ""
	if len(os.Args) >= 4 {
		serverAddress = os.Args[3]
	}

	saveTo := ""
	pieceId := 0

	if os.Args[1] == "download_piece" {
		fileName = os.Args[4]
		saveTo = os.Args[3]
		pieceId, _ = strconv.Atoi(os.Args[5])
	}
	if os.Args[1] == "download" {
		fileName = os.Args[4]
		saveTo = os.Args[3]
	}
	if command == "decode" {
		bencodedValue := os.Args[2]
		fmt.Println(decodeString(bencodedValue))
	} else if command == "info" {
		fillInfo(fileName)
		fmt.Printf("Tracker URL: %s\nLength: %d\nInfo Hash: %s\nPiece Length: %d\nPiece Hashes:%s\n", tracker, fileLength, infoHash, pieceLength, piecesHash)
	} else if command == "peers" {
		fillInfo(fileName)
		makeRequest()
		fmt.Println(peers)
	} else if command == "handshake" {
		fillInfo(fileName)
		conn, _ := net.Dial("tcp", serverAddress)
		defer conn.Close()
		conn.Write(getHandShakeMessage())
		buffer := make([]byte, 100)
		conn.Read(buffer)
		fmt.Printf("Peer ID: %s\n", getHexValue(buffer[48:68]))
	} else if command == "download_piece" {
		fillInfo(fileName)
		makeRequest()
		conn := getConnection(0)
		defer conn.Close()
		createAndSaveFile(getPieceBytes(conn, pieceId), saveTo)
		fmt.Printf("Piece %d downloaded to %s.", pieceId, saveTo)
	} else if command == "download" {
		fillInfo(fileName)
		makeRequest()
		var combinedPieces []byte
		for i := 0; i < pieceCount; i++ {
			conn := getConnection(0)
			combinedPieces = append(combinedPieces, getPieceBytes(conn, i)...)
			conn.Close()
		}
		createAndSaveFile(combinedPieces, saveTo)
		fmt.Printf("Downloaded %s to %s.", fileName, saveTo)
	} else {
		fmt.Println("Unknown command: " + command)
		os.Exit(1)
	}
}
