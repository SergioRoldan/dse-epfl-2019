package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	"go.dedis.ch/protobuf"
)

/* GOSSIPER STRUCT & WORKER */

// NewGossiper given the address, the name, the peers and the mode returns the pointer to a new created Gossiper
func NewGossiper(address, name, peers string, mode, hw3ex2, hw3ex3, hw3ex4, ackAll bool, N, stubbornTout int, hopLimit uint) *Gossiper {

	// Resolve address and listen UDP
	udpAddr, addrErr := net.ResolveUDPAddr("udp4", address)

	if addrErr != nil {
		fmt.Println("Addr Err: " + addrErr.Error())
	}

	udpConn, connErr := net.ListenUDP("udp4", udpAddr)

	if connErr != nil {
		fmt.Println("Conn err: " + connErr.Error())
		return nil
	}

	// Generate ID
	id := name

	// Define status
	status := StatusPacket{}
	status.Want = append(status.Want, PeerStatus{Identifier: id, NextID: 1})

	// Initialize Mutex
	mutexs := Mutexs{
		peersMutex:          &sync.Mutex{},
		rumorsToAckMutex:    &sync.Mutex{},
		statusMutex:         &sync.Mutex{},
		rumorsMutex:         &sync.Mutex{},
		routingMutex:        &sync.Mutex{},
		filesIndexMutex:     &sync.Mutex{},
		privateMutex:        &sync.Mutex{},
		downloadsMutex:      &sync.Mutex{},
		searchsMutex:        &sync.Mutex{},
		recentSearchsMutex:  &sync.Mutex{},
		blocksMutex:         &sync.Mutex{},
		gossipWithConfMutex: &sync.Mutex{},
		tlcAcksMutex:        &sync.Mutex{},
		tlcMessagesMutex:    &sync.Mutex{},
		searchResultMutex:   &sync.Mutex{},
	}

	// Initialize hw3 and myTime
	hw3 := hw3{
		hw3ex2:       hw3ex2,
		hw3ex3:       hw3ex3,
		hw3ex4:       hw3ex4,
		ackAll:       ackAll,
		N:            N,
		stubbornTout: stubbornTout,
		hopLimit:     uint32(hopLimit),
	}

	myTime := uint32(0)

	// Return the Gossiper pointer
	return &Gossiper{
		address:        udpAddr,
		conn:           udpConn,
		Name:           name,
		peers:          peers,
		Status:         status,
		simpleMode:     mode,
		ID:             id,
		myTime:         &myTime,
		hw3:            hw3,
		rumors:         make(map[string][]GossipPacket),
		rumorsToAck:    make(map[string][]RumorAck),
		rumorsAcked:    make(map[string][]RumorAck),
		tlcMessages:    make(map[string][]TLCMessage),
		unconfirmedTLC: make(map[string][]TLCMessage),
		routingTable:   make(map[string]string),
		filesIndex:     make(map[[32]byte]FileIndex),
		private:        make(map[string][]PrivateMessage),
		SearchResult:   make(map[string]SearchMatch),
		recentSearchs:  make(map[string][][]string),
		mutexs:         mutexs,
	}
}

// Defines a new gossiper worker that listen for incoming messages in Gossiper connection
func gossipWorker(gos *Gossiper) {
	fmt.Println("New gossiper worker with name " + gos.Name + " started at " + gos.address.IP.String() + ":" + strconv.Itoa(gos.address.Port))

	// Defer the close of the connection
	defer gos.conn.Close()

	// Listen for new connections indefinetly
	for {
		handleGossiperConnection(gos.conn, gos)
	}
}

/* GOSSIPER HANDLERS */
// Handle a rumor message
func handleRumor(addr string, gos *Gossiper, msg *GossipPacket) {
	var origin *PeerStatus
	var originPos int

	var msgOrigin string
	var msgID uint32

	if msg.TLCMessage != nil {
		msgOrigin = msg.TLCMessage.Origin
		msgID = msg.TLCMessage.ID
	} else {
		msgOrigin = msg.Rumor.Origin
		msgID = msg.Rumor.ID
	}

	// If node is unknown add to the status list and update the routing table
	gos.mutexs.statusMutex.Lock()
	for i, n := range gos.Status.Want {
		if n.Identifier == msgOrigin {
			origin = &n
			originPos = i
			break
		}
	}

	if origin == nil {
		origin = &PeerStatus{msgOrigin, 1}
		gos.Status.Want = append(gos.Status.Want, *origin)
		originPos = len(gos.Status.Want) - 1
	}
	gos.mutexs.statusMutex.Unlock()

	// If rumor is a new rumor: update state if we were expecting this message; store and ack the rumor of the sender and start rumormongering with a random peer; also update the routing table
	found := false

	gos.mutexs.rumorsMutex.Lock()
	for _, rum := range gos.rumors[origin.Identifier] {
		if (rum.TLCMessage != nil && rum.TLCMessage.ID == msgID) || (rum.Rumor != nil && rum.Rumor.ID == msgID) {
			found = true
			break
		}
	}
	gos.mutexs.rumorsMutex.Unlock()

	if !found {
		if msgID >= origin.NextID {
			updateRoutingEntry(addr, msgOrigin, gos, (msg.Rumor != nil && msg.Rumor.Text != "") || false)
		}

		if msgID == origin.NextID {
			gos.mutexs.rumorsMutex.Lock()
			gos.mutexs.statusMutex.Lock()
			gos.Status.Want[originPos].NextID++

			for _, r := range gos.rumors[origin.Identifier] {
				var rID uint32

				if r.TLCMessage != nil {
					rID = r.TLCMessage.ID
				} else {
					rID = r.Rumor.ID
				}

				if rID == gos.Status.Want[originPos].NextID {
					gos.Status.Want[originPos].NextID++
				}
			}
			gos.mutexs.rumorsMutex.Unlock()
			gos.mutexs.statusMutex.Unlock()
		}

		storeNewMsg(gos, msg, addr)
		sendStatusTo(addr, *gos)

		rndPeer := randPeer(*gos)
		rumormongering(gos, msg, rndPeer)
		// If rumor is something we have just ackowledge it
	} else {
		sendStatusTo(addr, *gos)
	}
}

// Handle a status message
func handleStatus(addr string, gos *Gossiper, msg *GossipPacket) {
	printMessage(*msg, gos.simpleMode, false, addr)
	gos.mutexs.peersMutex.Lock()
	printPeers(gos.peers)
	gos.mutexs.peersMutex.Unlock()

	var msgToSend *PeerStatus
	var msgToReceive bool
	var msgToAck []RumorAck

	// Check if we already have all the nodes of the status received
	for _, msgStatus := range msg.Status.Want {
		isInStatus := false

		gos.mutexs.statusMutex.Lock()
		for _, gosStatus := range gos.Status.Want {
			// If we already have the node in our status
			if gosStatus.Identifier == msgStatus.Identifier {
				isInStatus = true
				// Check if our status is higher than the sender's status
				if gosStatus.NextID > msgStatus.NextID && msgToSend == nil {
					msgToSend = &PeerStatus{Identifier: msgStatus.Identifier, NextID: msgStatus.NextID}
					// Otherwise check if sender's status is higher than ours
				} else if gosStatus.NextID < msgStatus.NextID {
					msgToReceive = true
				}

				// Check all the messages acked by this status
				gos.mutexs.rumorsToAckMutex.Lock()
				for j := 0; j < len(gos.rumorsToAck[addr]); j++ {
					if gos.rumorsToAck[addr][j].Origin == msgStatus.Identifier && msgStatus.NextID >= gos.rumorsToAck[addr][j].ID {
						msgToAck = append(msgToAck, gos.rumorsToAck[addr][j])
						gos.rumorsToAck[addr] = append(gos.rumorsToAck[addr][:j], gos.rumorsToAck[addr][j+1:]...)
					}
				}
				gos.mutexs.rumorsToAckMutex.Unlock()

				break
			}
		}

		// Add new nodes to our status and if NextID is bigger than 1 define message to receive as true
		if !isInStatus {
			gos.Status.Want = append(gos.Status.Want, PeerStatus{Identifier: msgStatus.Identifier, NextID: 1})

			if msgStatus.NextID > 1 {
				msgToReceive = true
			}
		}
		gos.mutexs.statusMutex.Unlock()

	}

	// If I have a message to send: start rumormongering with the sender and this message
	if msgToSend != nil {
		gos.mutexs.rumorsMutex.Lock()
		gosRumors := gos.rumors[msgToSend.Identifier]
		gos.mutexs.rumorsMutex.Unlock()

		for _, rm := range gosRumors {
			var rmID uint32

			if rm.Rumor != nil {
				rmID = rm.Rumor.ID
			} else {
				rmID = rm.TLCMessage.ID
			}

			if msgToSend.NextID == rmID {
				gossPacket := &GossipPacket{}

				if rm.Rumor != nil {
					r := &RumorMessage{Origin: rm.Rumor.Origin, ID: rm.Rumor.ID, Text: rm.Rumor.Text}
					gossPacket.Rumor = r
				} else {
					r := &TLCMessage{Origin: rm.TLCMessage.Origin, ID: rm.TLCMessage.ID,
						Confirmed: rm.TLCMessage.Confirmed, TxBlock: rm.TLCMessage.TxBlock,
						VectorClock: rm.TLCMessage.VectorClock, Fitness: rm.TLCMessage.Fitness}
					gossPacket.TLCMessage = r
				}

				rumormongering(gos, gossPacket, addr)

				break
			}
		}
		storeAckedMessage(gos, msgToAck, addr)
		// Otherwise if I have a message to receive: send a status to the sender
	} else if msgToReceive {
		sendStatusTo(addr, *gos)
		storeAckedMessage(gos, msgToAck, addr)
		// Otherwise we are in sync with sender
	} else {
		printInSync(addr)

		// For each message acked throw a coin and delete all acked messages
		msgToAck = append(msgToAck, gos.rumorsAcked[addr]...)
		gos.rumorsAcked[addr] = gos.rumorsAcked[addr][:0]

		for _, flipCoin := range msgToAck {

			if rand.Int()%2 == 0 {
				gos.mutexs.rumorsMutex.Lock()
				gosRumors := gos.rumors[flipCoin.Origin]
				gos.mutexs.rumorsMutex.Unlock()
				for _, rm := range gosRumors {
					var rmID uint32

					if rm.Rumor != nil {
						rmID = rm.Rumor.ID
					} else {
						rmID = rm.TLCMessage.ID
					}

					if flipCoin.ID == rmID {
						gossPacket := &GossipPacket{}

						if rm.Rumor != nil {
							r := &RumorMessage{Origin: rm.Rumor.Origin, ID: rm.Rumor.ID, Text: rm.Rumor.Text}
							gossPacket.Rumor = r
						} else {
							r := &TLCMessage{Origin: rm.TLCMessage.Origin, ID: rm.TLCMessage.ID,
								Confirmed: rm.TLCMessage.Confirmed, TxBlock: rm.TLCMessage.TxBlock,
								VectorClock: rm.TLCMessage.VectorClock, Fitness: rm.TLCMessage.Fitness}
							gossPacket.TLCMessage = r
						}

						rndPeer := randPeer(*gos)
						printFlippedCoin(rndPeer)
						rumormongering(gos, gossPacket, rndPeer)

						break
					}
				}
			}
		}
	}
}

// Handle a private message
func handlePrivateMsg(gos *Gossiper, msg *PrivateMessage, isTLCAck bool) {
	// If the message is for me print it and add it to the private list of origin's node
	if !isTLCAck && gos.ID == msg.Destination {
		printPrivateMessage(*msg)
		gos.mutexs.privateMutex.Lock()
		gos.private[msg.Origin] = append(gos.private[msg.Origin], *msg)
		gos.mutexs.privateMutex.Unlock()
		return
	} else if isTLCAck && gos.ID == msg.Destination {
		gos.mutexs.tlcAcksMutex.Lock()
		for _, c := range gos.tlcAcks {
			if c != nil {
				tlcAck := TLCAck(*msg)
				c <- tlcAck
			}
		}
		gos.mutexs.tlcAcksMutex.Unlock()
		return
	}

	// Discard the message silenty if the hop limit has been reached
	if msg.HopLimit == 0 {
		return
	}

	// Get the routing address and forward the private message
	gos.mutexs.routingMutex.Lock()
	addr := gos.routingTable[msg.Destination]
	gos.mutexs.routingMutex.Unlock()

	if addr != "" {
		msgTmp := &GossipPacket{}

		rmr := PrivateMessage{
			Origin:      msg.Origin,
			ID:          msg.ID,
			Text:        msg.Text,
			Destination: msg.Destination,
			HopLimit:    msg.HopLimit - 1,
		}

		if !isTLCAck {
			msgTmp.Private = &rmr
		} else {
			tlcAck := TLCAck(rmr)
			msgTmp.Ack = &tlcAck
		}

		sendMsgTo(addr, msgTmp, *gos)
	} else {
		fmt.Println("Unable to send message to unknown peer")
	}
}

// Handle a data request message
func handleDataRequest(gos *Gossiper, msg *GossipPacket) {
	// Check if the request is for me
	if gos.ID == msg.DataRequest.Destination {
		var hashVal [32]byte

		copy(hashVal[:], msg.DataRequest.HashValue)
		// If the hash is a metahash send the meta file
		gos.mutexs.filesIndexMutex.Lock()
		defer gos.mutexs.filesIndexMutex.Unlock()
		if _, ok := gos.filesIndex[hashVal]; ok {
			rmr := &DataReply{
				Origin:      gos.ID,
				Destination: msg.DataRequest.Origin,
				HopLimit:    HOPLIMIT - 1,
				HashValue:   msg.DataRequest.HashValue,
				Data:        gos.filesIndex[hashVal].Meta,
			}

			gos.mutexs.routingMutex.Lock()
			addr := gos.routingTable[msg.DataRequest.Origin]
			gos.mutexs.routingMutex.Unlock()

			if addr != "" {
				msgTmp := &GossipPacket{DataReply: rmr}

				sendMsgTo(addr, msgTmp, *gos)
			} else {
				fmt.Println("Unable to send message to unknown peer")
				return
			}
		} else {
			// If the hash is not a metahash check if it's a chunk of an indexed file and send the corresponding chunk
			fnd := false
			for _, v := range gos.filesIndex {
				if bytes.Contains(v.Meta, msg.DataRequest.HashValue) {
					f, err := os.Open(CHUNKDIR + hex.EncodeToString(msg.DataRequest.HashValue) + ".part")

					rmr := &DataReply{}

					if err != nil {
						rmr = &DataReply{
							Origin:      gos.ID,
							Destination: msg.DataRequest.Origin,
							HopLimit:    HOPLIMIT - 1,
							HashValue:   msg.DataRequest.HashValue,
						}
					} else {
						b, er := ioutil.ReadAll(f)

						if er != nil {
							fmt.Println("Unable to read file")
						}

						rmr = &DataReply{
							Origin:      gos.ID,
							Destination: msg.DataRequest.Origin,
							HopLimit:    HOPLIMIT - 1,
							HashValue:   msg.DataRequest.HashValue,
							Data:        b,
						}
					}

					f.Close()

					gos.mutexs.routingMutex.Lock()
					addr := gos.routingTable[msg.DataRequest.Origin]
					gos.mutexs.routingMutex.Unlock()

					if addr != "" {
						msgTmp := &GossipPacket{DataReply: rmr}

						sendMsgTo(addr, msgTmp, *gos)
					} else {
						fmt.Println("Unable to send message to unknown peer")
						return
					}

					fnd = true
					break
				}
			}

			// If it's not send and empty data reply
			if !fnd {
				rmr := &DataReply{
					Origin:      gos.ID,
					Destination: msg.DataRequest.Origin,
					HopLimit:    HOPLIMIT - 1,
					HashValue:   msg.DataRequest.HashValue,
				}

				msgTmp := &GossipPacket{DataReply: rmr}

				gos.mutexs.routingMutex.Lock()
				addr := gos.routingTable[msg.DataRequest.Origin]
				gos.mutexs.routingMutex.Unlock()

				if addr != "" {
					sendMsgTo(addr, msgTmp, *gos)
				} else {
					fmt.Println("Unable to send message to unknown peer")
				}
			}
		}

		return
	}

	if msg.DataRequest.HopLimit == 0 {
		return
	}

	gos.mutexs.routingMutex.Lock()
	addr := gos.routingTable[msg.DataRequest.Destination]
	gos.mutexs.routingMutex.Unlock()

	// If it's not forward the request
	if addr != "" {
		rmr := &DataRequest{
			Origin:      msg.DataRequest.Origin,
			Destination: msg.DataRequest.Destination,
			HopLimit:    msg.DataRequest.HopLimit - 1,
			HashValue:   msg.DataRequest.HashValue,
		}

		msgTmp := &GossipPacket{DataRequest: rmr}

		sendMsgTo(addr, msgTmp, *gos)
	} else {
		fmt.Println("Unable to send message to unknown peer")
	}
}

// Handle a data reply message
func handleDataReply(gos *Gossiper, msg *GossipPacket) {
	// If it's for me notify all the active download threads of the new reply using its channels
	if gos.ID == msg.DataReply.Destination {
		gos.mutexs.downloadsMutex.Lock()
		for _, c := range gos.downloads {
			c <- *msg.DataReply
		}
		gos.mutexs.downloadsMutex.Unlock()
		return
	}

	if msg.DataReply.HopLimit == 0 {
		return
	}

	gos.mutexs.routingMutex.Lock()
	addr := gos.routingTable[msg.DataReply.Destination]
	gos.mutexs.routingMutex.Unlock()

	// If it's not forward the reply
	if addr != "" {
		rmr := &DataReply{
			Origin:      msg.DataReply.Origin,
			Destination: msg.DataReply.Destination,
			HopLimit:    msg.DataReply.HopLimit - 1,
			HashValue:   msg.DataReply.HashValue,
			Data:        msg.DataReply.Data,
		}

		msgTmp := &GossipPacket{DataReply: rmr}

		sendMsgTo(addr, msgTmp, *gos)
	} else {
		fmt.Println("Unable to send message to unknown peer")
	}
}

// Handle a search request checking if the request is duplicated
func handleSearchRequest(addr string, gos *Gossiper, msg *GossipPacket) {
	sortKeywords := msg.SearchRequest.Keywords
	sort.Strings(sortKeywords)

	gos.mutexs.recentSearchsMutex.Lock()
	recentSearchs := gos.recentSearchs[msg.SearchRequest.Origin]
	gos.mutexs.recentSearchsMutex.Unlock()

	for _, keywords := range recentSearchs {
		if reflect.DeepEqual(keywords, sortKeywords) {
			return
		}
	}

	go handleDuplicateSearch(gos, msg.SearchRequest.Origin, sortKeywords)

	files := make(map[string]FileIndex)

	for _, fIndex := range gos.filesIndex {
		for _, keyword := range sortKeywords {
			if strings.Contains(fIndex.Name, keyword) {
				files[fIndex.Name] = fIndex
			}
		}
	}

	updateRoutingEntry(addr, msg.SearchRequest.Origin, gos, false)

	// Send the files matching the request
	if len(files) > 0 {

		searchReply := &SearchReply{
			Origin:      gos.ID,
			Destination: msg.SearchRequest.Origin,
			HopLimit:    HOPLIMIT - 1,
		}

		for _, file := range files {

			var meta []byte

			for _, b := range file.MetaHash[:] {
				meta = append(meta, b)
			}

			searchRes := SearchResult{
				FileName:     file.Name,
				ChunkMap:     file.ChunkMap,
				MetafileHash: meta,
				ChunkCount:   file.ChunkCount,
			}

			searchReply.Results = append(searchReply.Results, &searchRes)

		}

		sendNewSearchReply(gos, searchReply)
	}

	// Forward the request acordingly
	divideBudget(gos, msg.SearchRequest.Budget-1, msg.SearchRequest.Origin, addr, sortKeywords)
}

// Handle search reply
func handleSearchReply(gos *Gossiper, msg *GossipPacket) {
	// If it is for us notify the channels
	if gos.ID == msg.SearchReply.Destination {
		gos.mutexs.searchsMutex.Lock()
		defer gos.mutexs.searchsMutex.Unlock()
		for _, c := range gos.searchs {
			c <- *msg.SearchReply
		}
		return
	}

	if msg.SearchReply.HopLimit == 0 {
		return
	}

	gos.mutexs.routingMutex.Lock()
	addr := gos.routingTable[msg.SearchReply.Destination]
	gos.mutexs.routingMutex.Unlock()

	// If it's not forward the reply
	if addr != "" {
		rmr := &SearchReply{
			Origin:      msg.SearchReply.Origin,
			Destination: msg.SearchReply.Destination,
			HopLimit:    msg.SearchReply.HopLimit - 1,
			Results:     msg.SearchReply.Results,
		}

		msgTmp := &GossipPacket{SearchReply: rmr}

		sendMsgTo(addr, msgTmp, *gos)
	} else {
		fmt.Println("Unable to send message to unknown peer")
	}
}

// Gossiper handler: get the message and interact accordingly
func handleGossiperConnection(conn *net.UDPConn, gos *Gossiper) {
	buffer := make([]byte, 9216)

	l, ipAddr, readErr := conn.ReadFromUDP(buffer)

	if readErr != nil {
		fmt.Println("Read error: " + readErr.Error())
	}

	msg := &GossipPacket{}

	decodeErr := protobuf.Decode(buffer[0:l], msg)

	if decodeErr != nil {
		panic("Decode error: " + decodeErr.Error())
	}

	addr := ipAddr.IP.String() + ":" + strconv.Itoa(ipAddr.Port)

	// Add a new peer if the sender is unknown
	gos.mutexs.peersMutex.Lock()
	if !strings.Contains(gos.peers, addr) {
		if gos.peers == "" {
			gos.peers = addr
		} else {
			auxPeers := strings.Split(gos.peers, ",")
			auxPeers = append(auxPeers, addr)
			gos.peers = strings.Join(auxPeers, ",")
		}

	}
	gos.mutexs.peersMutex.Unlock()

	// In simple mode replace parameters and forward message
	if gos.simpleMode {
		printMessage(*msg, gos.simpleMode, false, "")
		gos.mutexs.peersMutex.Lock()
		forwardPeers := strings.Replace(gos.peers, addr, "", -1)
		gos.mutexs.peersMutex.Unlock()
		msg.Simple.RelayPeerAddr = gos.address.IP.String() + ":" + strconv.Itoa(gos.address.Port)
		broadcast(forwardPeers, msg, *gos)
		gos.mutexs.peersMutex.Lock()
		printPeers(gos.peers)
		gos.mutexs.peersMutex.Unlock()
		// If it's another kind of message let its handler take care of it
	} else if msg.Rumor != nil {
		handleRumor(addr, gos, msg)
	} else if msg.Status != nil {
		handleStatus(addr, gos, msg)
	} else if msg.Private != nil {
		handlePrivateMsg(gos, msg.Private, false)
	} else if msg.Ack != nil {
		// Cast TLCAck to PrivateMessage to use the same handler
		tlcAck := PrivateMessage(*msg.Ack)
		handlePrivateMsg(gos, &tlcAck, true)
	} else if msg.DataRequest != nil {
		handleDataRequest(gos, msg)
	} else if msg.DataReply != nil {
		handleDataReply(gos, msg)
	} else if msg.SearchRequest != nil {
		handleSearchRequest(addr, gos, msg)
	} else if msg.SearchReply != nil {
		handleSearchReply(gos, msg)
	} else if msg.TLCMessage != nil {
		// Depending on the ex handle a TLCMessage including: handle Status, handle Rumor, acknowledge TLC, increment myTime and handle out of order TLCs
		if gos.hw3.hw3ex3 || gos.hw3.hw3ex4 {
			msgTmp := GossipPacket{Status: msg.TLCMessage.VectorClock}
			handleStatus(addr, gos, &msgTmp)
		}

		handleRumor(addr, gos, msg)

		fnd := false
		position := -1
		gos.mutexs.tlcMessagesMutex.Lock()
		for j, tlcM := range gos.tlcMessages[msg.TLCMessage.Origin] {
			if msg.TLCMessage.ID == tlcM.ID || int(tlcM.ID) == msg.TLCMessage.Confirmed {
				fnd = true
				position = j
				break
			}
		}
		tlcMessageFromOrigin := gos.tlcMessages[msg.TLCMessage.Origin]
		gos.mutexs.tlcMessagesMutex.Unlock()

		if msg.TLCMessage.Confirmed == -1 {

			if !fnd {
				gos.mutexs.tlcMessagesMutex.Lock()
				gos.tlcMessages[msg.TLCMessage.Origin] = append(gos.tlcMessages[msg.TLCMessage.Origin], *msg.TLCMessage)
				gos.mutexs.tlcMessagesMutex.Unlock()
				printUnconfirmedTLC(*msg.TLCMessage)
			}

			gos.mutexs.tlcMessagesMutex.Lock()
			currentRound := uint32(*gos.myTime*2 + 1)
			gos.mutexs.tlcMessagesMutex.Unlock()

			if gos.hw3.hw3ex2 || gos.hw3.ackAll || msg.TLCMessage.ID == currentRound {
				if gos.hw3.hw3ex4 {
					if validBlockPublish(gos, *msg.TLCMessage) {
						ackTLC(gos, *msg.TLCMessage)
						confirmOutofOrderTLC(gos, *msg.TLCMessage)
					}
				} else {
					ackTLC(gos, *msg.TLCMessage)
					confirmOutofOrderTLC(gos, *msg.TLCMessage)
				}
			}
		} else {
			if !fnd {
				storeOutOfOrderTLC(gos, *msg.TLCMessage)
			} else if fnd && tlcMessageFromOrigin[position].Confirmed == -1 {
				gos.mutexs.tlcMessagesMutex.Lock()
				gos.tlcMessages[msg.TLCMessage.Origin][position].Confirmed = msg.TLCMessage.Confirmed
				gos.mutexs.tlcMessagesMutex.Unlock()
				printConfirmedTLC(gos, *msg.TLCMessage)
				if !gos.hw3.hw3ex2 {
					incrementMytime(gos)
				}
			}
		}
	}

}

// Get the message content, generate a GossipPacket and start rumormongering or broadcast depending on the mode
func handleClientConnection(conn *net.UDPConn, gos *Gossiper) {
	buffer := make([]byte, 8192)

	l, _, readErr := conn.ReadFromUDP(buffer)

	if readErr != nil {
		fmt.Println("Read error: " + readErr.Error())
	}

	tmpMsg := &Message{}
	protobuf.Decode(buffer[0:l], tmpMsg)

	// Depending on client message parameters send a simple message, a rumor, a private message, index a file or download a file
	if gos.simpleMode {
		sendNewSimpleMessage(gos, tmpMsg.Text)
	} else if tmpMsg.File != nil {
		if tmpMsg.Request != nil && tmpMsg.Destination != nil {
			ch := make(chan DataReply)
			gos.mutexs.downloadsMutex.Lock()
			gos.downloads = append(gos.downloads, ch)
			gos.mutexs.downloadsMutex.Unlock()
			go handleFileDownload(gos, *tmpMsg, ch)
		} else if tmpMsg.Request != nil {

			var searchMatch *SearchMatch

			gos.mutexs.searchResultMutex.Lock()
			for _, searchResult := range gos.SearchResult {
				if bytes.Compare(searchResult.MetafileHash, *tmpMsg.Request) == 0 {
					searchMatch = &searchResult
					break
				}
			}
			gos.mutexs.searchResultMutex.Unlock()

			if searchMatch == nil {
				fmt.Println("File with hash " + hex.EncodeToString(*tmpMsg.Request) + " not found in a previous search")
				return
			}

			ch := make(chan DataReply)
			gos.mutexs.downloadsMutex.Lock()
			gos.downloads = append(gos.downloads, ch)
			gos.mutexs.downloadsMutex.Unlock()
			go handleFileDownloadFromSearch(gos, searchMatch, *tmpMsg.File, ch)
		} else {
			handleFileIndexing(gos, *tmpMsg.File)
		}
	} else if tmpMsg.Destination != nil {
		sendNewPrivateMessage(gos, tmpMsg.Text, *tmpMsg.Destination)
	} else if tmpMsg.Keywords != nil {
		tmpKeywords := strings.Split(*tmpMsg.Keywords, ",")
		sort.Strings(tmpKeywords)

		ch := make(chan SearchReply)
		gos.mutexs.searchsMutex.Lock()
		gos.searchs = append(gos.searchs, ch)
		gos.mutexs.searchsMutex.Unlock()

		go newFileSearch(gos, tmpKeywords, tmpMsg.Budget, ch)
	} else {
		sendNewRumorMessage(gos, tmpMsg.Text, nil)
	}

}
