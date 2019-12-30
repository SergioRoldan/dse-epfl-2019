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
	Confirmed   int
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
	Rumors map[string][]GossipPacket
}

type RoundResponse struct {
	Round    int
	Advances []string
}

type SearchMatchResponse struct {
	SearchMatches []string
}

type ConsensusResponse struct {
	Consensus []string
}

type ConfirmedTLCResponse struct {
	Confirmeds []string
}

type PrivateResponse struct {
	Private []PrivateMessage
}

type FileSearchedResponse struct {
	FileSearched []SearchMatch
	Hashes []string
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
	peersMutex          *sync.Mutex
	rumorsToAckMutex    *sync.Mutex
	statusMutex         *sync.Mutex
	rumorsMutex         *sync.Mutex
	routingMutex        *sync.Mutex
	filesIndexMutex     *sync.Mutex
	privateMutex        *sync.Mutex
	downloadsMutex      *sync.Mutex
	searchsMutex        *sync.Mutex
	recentSearchsMutex  *sync.Mutex
	searchResultMutex   *sync.Mutex
	blocksMutex         *sync.Mutex
	gossipWithConfMutex *sync.Mutex
	tlcAcksMutex        *sync.Mutex
	tlcMessagesMutex    *sync.Mutex
}

type hw3 struct {
	hw3ex2       bool
	hw3ex3       bool
	hw3ex4       bool
	ackAll       bool
	N            int
	stubbornTout int
	hopLimit     uint32
}

type gossipWithConfChannel struct {
	round uint32
	ch    chan int
}

type Gossiper struct {
	address        *net.UDPAddr
	conn           *net.UDPConn
	myTime         *uint32
	Name           string
	ID             string
	peers          string
	simpleMode     bool
	hw3            hw3
	Status         StatusPacket
	mutexs         Mutexs
	routingTable   map[string]string
	SearchResult   map[string]SearchMatch
	rumors         map[string][]GossipPacket
	rumorsToAck    map[string][]RumorAck
	rumorsAcked    map[string][]RumorAck
	private        map[string][]PrivateMessage
	tlcMessages    map[string][]TLCMessage
	unconfirmedTLC map[string][]TLCMessage
	recentSearchs  map[string][][]string
	filesIndex     map[[32]byte]FileIndex
	searchMatches  []string
	roundAdvances  []string
	confirmedTLCs  []string
	consensusOn	   []string
	downloads      []chan DataReply
	searchs        []chan SearchReply
	tlcAcks        []chan TLCAck
	blocks         []TLCMessage
	gossipWithConf []gossipWithConfChannel
}
