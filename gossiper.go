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
func NewGossiper(address, name, peers string, mode bool, N, stubbornTout int, hopLimit uint) *Gossiper {

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
	/* The following line appends a 4 digits hexa to the ID to make it unique but is commented to pass the test 2 */
	//id := name + fmt.Sprintf("%x", rand.Intn(65535))

	// Define status
	status := StatusPacket{}
	status.Want = append(status.Want, PeerStatus{Identifier: id, NextID: 1})

	mutexs := Mutexs{
		peersMutex:       &sync.Mutex{},
		rumorsToAckMutex: &sync.Mutex{},
		statusMutex:      &sync.Mutex{},
		rumorsMutex:      &sync.Mutex{},
		routingMutex:     &sync.Mutex{},
		filesIndexMutex:  &sync.Mutex{},
		privateMutex:     &sync.Mutex{},
		downloadsMutex:   &sync.Mutex{},
	}

	// Return the Gossiper pointer
	return &Gossiper{
		address:      udpAddr,
		conn:         udpConn,
		Name:         name,
		peers:        peers,
		Status:       status,
		simpleMode:   mode,
		ID:           id,
		N:            N,
		stubbornTout: stubbornTout,
		hopLimit:     uint32(hopLimit),
		rumors:       make(map[string][]RumorMessage),
		rumorsToAck:  make(map[string][]RumorAck),
		rumorsAcked:  make(map[string][]RumorAck),
		routingTable: make(map[string]string),
		filesIndex:   make(map[[32]byte]FileIndex),
		private:      make(map[string][]PrivateMessage),
		SearchResult: make(map[string]SearchMatch),
		mutexs:       mutexs,
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

	// If node is unknown add to the status list and update the routing table
	gos.mutexs.statusMutex.Lock()
	for i, n := range gos.Status.Want {
		if n.Identifier == msg.Rumor.Origin {
			origin = &n
			originPos = i
			break
		}
	}

	if origin == nil {
		origin = &PeerStatus{msg.Rumor.Origin, 1}
		gos.Status.Want = append(gos.Status.Want, *origin)
		originPos = len(gos.Status.Want) - 1
	}
	gos.mutexs.statusMutex.Unlock()

	// If rumor is a new rumor: update state if we were expecting this message; store and ack the rumor of the sender and start rumormongering with a random peer; also update the routing table
	found := false

	gos.mutexs.rumorsMutex.Lock()
	for _, rum := range gos.rumors[origin.Identifier] {
		if rum.ID == msg.Rumor.ID {
			found = true
			break
		}
	}
	gos.mutexs.rumorsMutex.Unlock()

	if !found {
		if msg.Rumor.ID >= origin.NextID {
			updateRoutingEntry(addr, msg.Rumor.Origin, gos, msg.Rumor.Text != "")
		}

		if msg.Rumor.ID == origin.NextID {
			gos.mutexs.statusMutex.Lock()
			gos.mutexs.rumorsMutex.Lock()
			gos.Status.Want[originPos].NextID++

			for _, r := range gos.rumors[origin.Identifier] {
				if r.ID == gos.Status.Want[originPos].NextID {
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
			if msgToSend.NextID == rm.ID {
				r := &RumorMessage{Origin: rm.Origin, ID: rm.ID, Text: rm.Text}
				gossPacket := &GossipPacket{Rumor: r}
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
					if flipCoin.ID == rm.ID {
						r := &RumorMessage{Origin: rm.Origin, ID: rm.ID, Text: rm.Text}
						gossPacket := &GossipPacket{Rumor: r}
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
		for _, c := range gos.tlcAcks {
			tlcAck := TLCAck(*msg)
			c <- tlcAck
		}
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
				HopLimit:    gos.hopLimit - 1,
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
							HopLimit:    gos.hopLimit - 1,
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
							HopLimit:    gos.hopLimit - 1,
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
					HopLimit:    gos.hopLimit - 1,
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

func handleSearchRequest(addr string, gos *Gossiper, msg *GossipPacket) {
	sortKeywords := msg.SearchRequest.Keywords
	sort.Strings(sortKeywords)

	//call to handle duplicate search with a map of recent requests
	for _, keywords := range gos.recentSearchs[msg.SearchRequest.Origin] {
		if reflect.DeepEqual(keywords, sortKeywords) {
			return
		}
	}

	go handleDuplicateSearch(gos, msg.SearchRequest.Origin, sortKeywords)

	files := make(map[[32]byte]FileIndex)

	for fHash, fIndex := range gos.filesIndex {
		for _, keyword := range sortKeywords {
			if strings.Contains(fIndex.Name, keyword) {
				files[fHash] = fIndex
			}
		}
	}

	if len(files) > 0 {

		searchReply := &SearchReply{
			Origin:      gos.ID,
			Destination: msg.SearchRequest.Origin,
			HopLimit:    gos.hopLimit - 1,
		}

		for _, file := range files {
			metaFile := file.MetaHash[:32]

			searchResult := &SearchResult{
				FileName:     file.Name,
				MetafileHash: metaFile,
				ChunkMap:     file.ChunkMap,
				ChunkCount:   file.ChunkCount,
			}

			searchReply.Results = append(searchReply.Results, searchResult)
		}

		sendNewSearchReply(gos, searchReply)
	}

	divideBudget(gos, msg.SearchRequest.Budget-1, msg.SearchRequest.Origin, addr, sortKeywords)
}

func handleSearchReply(gos *Gossiper, msg *GossipPacket) {
	if gos.ID == msg.SearchReply.Destination {
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

			for _, searchResult := range gos.SearchResult {
				if bytes.Compare(searchResult.MetafileHash, *tmpMsg.Request) == 0 {
					searchMatch = &searchResult
					break
				}
			}

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
		gos.searchs = append(gos.searchs, ch)

		go newFileSearch(gos, tmpKeywords, tmpMsg.Budget, ch)
	} else {
		sendNewRumorMessage(gos, tmpMsg.Text, nil)
	}

}
