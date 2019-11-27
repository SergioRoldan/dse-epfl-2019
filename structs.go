package main

import (
	"net"
	"sync"
)

/* STRUCTS */

type Message struct {
	Text        string
	Destination *string
	File        *string
	Request     *[]byte
	Keywords    *string
	Budget      *uint64
}

type TxPublish struct {
	Name         string
	Size         int64
	MetafileHash []byte
}

type BlockPublish struct {
	PrevHash    [32]byte //(used in Exercise 4, for now 0)
	Transaction TxPublish
}

type DataRequest struct {
	Origin      string
	Destination string
	HopLimit    uint32
	HashValue   []byte
}

type DataReply struct {
	Origin      string
	Destination string
	HopLimit    uint32
	HashValue   []byte
	Data        []byte
}

type PrivateMessage struct {
	Origin      string
	ID          uint32
	Text        string
	Destination string
	HopLimit    uint32
}

type SimpleMessage struct {
	OriginalName  string
	RelayPeerAddr string
	Contents      string
}

type RumorMessage struct {
	Origin string
	ID     uint32
	Text   string
}

type PeerStatus struct {
	Identifier string
	NextID     uint32
}

type StatusPacket struct {
	Want []PeerStatus
}

type SearchRequest struct {
	Origin   string
	Budget   uint64
	Keywords []string
}

type SearchReply struct {
	Origin      string
	Destination string
	HopLimit    uint32
	Results     []*SearchResult
}

type SearchResult struct {
	FileName     string
	MetafileHash []byte
	ChunkMap     []uint64
	ChunkCount   uint64
}

type SearchMatch struct {
	FileName     string
	MetafileHash []byte
	ChunkCount   uint64
	Nodes        []string
	Chunks       []uint64
}

type TLCMessage struct {
	Origin      string
	ID          uint32
	Confirmed   bool
	TxBlock     BlockPublish
	VectorClock *StatusPacket //(used in Exercise 3, for now nil)
	Fitness     float32       //(used in Exercise 4, for now 0)
}

type TLCAck PrivateMessage

type GossipPacket struct {
	Simple        *SimpleMessage
	Rumor         *RumorMessage
	Status        *StatusPacket
	Private       *PrivateMessage
	DataRequest   *DataRequest
	DataReply     *DataReply
	SearchRequest *SearchRequest
	SearchReply   *SearchReply
	TLCMessage    *TLCMessage
	Ack           *TLCAck
}

type IDResponse struct {
	ID string
}

type NodesResponse struct {
	Peers string
}

type MessagesResponse struct {
	Rumors map[string][]RumorMessage
}

type SearchMatchResponse struct {
	SearchMatches []string
}

type PrivateResponse struct {
	Private []PrivateMessage
}

type FileSearchedResponse struct {
	FileSearched []SearchMatch
}

type RumorAck struct {
	Origin string
	ID     uint32
}

type FileIndex struct {
	Name       string
	Size       int64
	Meta       []byte
	MetaHash   [32]byte
	ChunkCount uint64
	ChunkMap   []uint64
}

type Mutexs struct {
	peersMutex       *sync.Mutex
	rumorsToAckMutex *sync.Mutex
	statusMutex      *sync.Mutex
	rumorsMutex      *sync.Mutex
	routingMutex     *sync.Mutex
	filesIndexMutex  *sync.Mutex
	privateMutex     *sync.Mutex
	downloadsMutex   *sync.Mutex
}

type Gossiper struct {
	address       *net.UDPAddr
	conn          *net.UDPConn
	Name          string
	peers         string
	ID            string
	Status        StatusPacket
	simpleMode    bool
	N             int
	stubbornTout  int
	hopLimit      uint32
	rumors        map[string][]RumorMessage
	rumorsToAck   map[string][]RumorAck
	rumorsAcked   map[string][]RumorAck
	private       map[string][]PrivateMessage
	recentSearchs map[string][][]string
	routingTable  map[string]string
	SearchResult  map[string]SearchMatch
	filesIndex    map[[32]byte]FileIndex
	downloads     []chan DataReply
	searchs       []chan SearchReply
	tlcAcks       []chan TLCAck
	searchMatches []string
	mutexs        Mutexs
}
