package main

import (
	"net"
	"sync"
)
/* STRUCTS */

// Message structure with Text string, Destination and File *strings; and Request *[]byte
type Message struct {
	Text string
	Destination *string
	File *string
	Request *[]byte
}

// DataRequest structure with Origin and Destination strings, HopLimit uint32 and HashValue []byte
type DataRequest struct {
	Origin string
	Destination string
	HopLimit uint32
	HashValue []byte
}

// DataReply structure with Origin and Destination strings, HopLimit uint32; HashValue and Data []bytes
type DataReply struct {
	Origin string
	Destination string
	HopLimit uint32
	HashValue []byte
	Data []byte
}

// PrivateMessage structure with Origin, Destination and Text strings; HopLimit and ID uints32
type PrivateMessage struct {
	Origin string
	ID uint32
	Text string
	Destination string
	HopLimit uint32
}

// SimpleMessage structure with OriginalName, RelayPeerAddr and Contents strings
type SimpleMessage struct {
	OriginalName  string
	RelayPeerAddr string
	Contents      string
}

// RumorMessage structure with Text, Origins strings and ID uint32
type RumorMessage struct {
	Origin string
	ID     uint32
	Text   string
}

// PeerStatus structure with Identifier string and NextID uint32
type PeerStatus struct {
	Identifier string
	NextID     uint32
}

// StatusPacket structure with Want slice of PeerStatus
type StatusPacket struct {
	Want []PeerStatus
}

// GossipPacket with a Simple *SimpleMessage
type GossipPacket struct {
	Simple *SimpleMessage
	Rumor  *RumorMessage
	Status *StatusPacket
	Private *PrivateMessage
	DataRequest *DataRequest
	DataReply *DataReply
}

// IDResponse structure with ID string
type IDResponse struct {
	ID string
}

// NodesResponse structure with Peers string
type NodesResponse struct {
	Peers string
}

// MessagesResponse structure with Rumors map[string][]RumorMessage
type MessagesResponse struct {
	Rumors map[string][]RumorMessage
}

// PrivateResponse structure with Private []PrivateMessage
type PrivateResponse struct {
	Private []PrivateMessage
}

// RumorAck structure with Origin string and ID uint32
type RumorAck struct {
	Origin string
	ID     uint32
}

// FileIndex structure with Name string, Size int64, Meta []byte and MetaHash [32]byte
type FileIndex struct {
	Name string
	Size int64
	Meta []byte
	MetaHash [32]byte
}

// Gossiper structure with address *net.UDPAddr, conn *net.UDPConn; Name, peers, ID strings; Sttus StatusPacket, simpleMode bool, rumors map[string][]RumorMessage; rumorsToAck
// rumorsAcked map[string][]RumorAck; peersMutex, rumorsToAckMutex, statusMutex, rumorsMutex, routingMutex, filesIndexMutex, privateMutex and downloadMutex *sync.Mutexs; routingTable map[string]string
// filesIndex map[[32]byte]FileIndex, private map[string][]PrivateMessage and downloads []chan DataReply
type Gossiper struct {
	address     *net.UDPAddr
	conn        *net.UDPConn
	Name        string
	peers       string
	Status      StatusPacket
	simpleMode  bool
	ID          string
	rumors      map[string][]RumorMessage
	rumorsToAck map[string][]RumorAck
	rumorsAcked map[string][]RumorAck
	routingTable map[string]string
	filesIndex map[[32]byte]FileIndex
	private map[string][]PrivateMessage
	downloads []chan DataReply
	peersMutex  *sync.Mutex
	rumorsToAckMutex *sync.Mutex
	statusMutex *sync.Mutex
	rumorsMutex *sync.Mutex
	routingMutex  *sync.Mutex
	filesIndexMutex *sync.Mutex
	privateMutex *sync.Mutex
	downloadsMutex *sync.Mutex
}
