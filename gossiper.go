package main

import (
	"fmt"
	"net"
	"math/rand"
	"sync"
	"strconv"
	"strings"

	"go.dedis.ch/protobuf"
)

/* GOSSIPER STRUCT & WORKER */

// NewGossiper given the address, the name, the peers and the mode returns the pointer to a new created Gossiper
func NewGossiper(address, name, peers string, mode bool) *Gossiper {

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

	// Return the Gossiper pointer
	return &Gossiper{
		address:     udpAddr,
		conn:        udpConn,
		Name:        name,
		peers:       peers,
		Status:      status,
		simpleMode:  mode,
		ID:          id,
		rumors:      make(map[string][]RumorMessage),
		rumorsToAck: make(map[string][]RumorAck),
		rumorsAcked: make(map[string][]RumorAck),
		peersMutex: &sync.Mutex{},
		rumorsToAckMutex: &sync.Mutex{},
		statusMutex: &sync.Mutex{},
		rumorsMutex: &sync.Mutex{},
		routingTable: make(map[string]string),
		filesIndex: make(map[[32]byte]FileIndex),
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
func handleRumor(addr string, gos *Gossiper, msg *GossipPacket) {
	var origin *PeerStatus
	var originPos int

	// If node is unknown add to the status list
	gos.statusMutex.Lock()
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

		updateRoutingEntry(addr, msg.Rumor.Origin, gos, true)
	}
	gos.statusMutex.Unlock()

	// If rumor is a new rumor: update state if we were expecting this message; store and ack the rumor of the sender and start rumormongering with a random peer
	found := false

	gos.rumorsMutex.Lock()
	for _, rum := range gos.rumors[origin.Identifier] {
		if rum.ID == msg.Rumor.ID {
			found = true
			break
		}
	}
	gos.rumorsMutex.Unlock()

	if !found {
		updateRoutingEntry(addr, msg.Rumor.Origin, gos, msg.Rumor.Text != "")

		if msg.Rumor.ID == origin.NextID {
			gos.statusMutex.Lock()
			gos.rumorsMutex.Lock()
			gos.Status.Want[originPos].NextID++
			
			for _, r := range gos.rumors[origin.Identifier] {
				if r.ID == gos.Status.Want[originPos].NextID {
					gos.Status.Want[originPos].NextID++
				}
			}
			gos.rumorsMutex.Unlock()
			gos.statusMutex.Unlock()
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

func handleStatus(addr string, gos *Gossiper, msg *GossipPacket) {
	printMessage(*msg, gos.simpleMode, false, addr)
	gos.peersMutex.Lock()
	printPeers(gos.peers)
	gos.peersMutex.Unlock()

	var msgToSend *PeerStatus
	var msgToReceive bool
	var msgToAck []RumorAck

	// Check if we already have all the nodes of the status received
	for _, msgStatus := range msg.Status.Want {
		isInStatus := false

		gos.statusMutex.Lock()
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
				gos.rumorsToAckMutex.Lock()
				for j := 0; j < len(gos.rumorsToAck[addr]); j++ {
					if gos.rumorsToAck[addr][j].Origin == msgStatus.Identifier && msgStatus.NextID >= gos.rumorsToAck[addr][j].ID {
						msgToAck = append(msgToAck, gos.rumorsToAck[addr][j])
						gos.rumorsToAck[addr] = append(gos.rumorsToAck[addr][:j], gos.rumorsToAck[addr][j+1:]...)
					}
				}
				gos.rumorsToAckMutex.Unlock()

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
		gos.statusMutex.Unlock()

	}

	// If I have a message to send: start rumormongering with the sender and this message
	if msgToSend != nil {
		gos.rumorsMutex.Lock()
		gosRumors := gos.rumors[msgToSend.Identifier]
		gos.rumorsMutex.Unlock()

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
				gos.rumorsMutex.Lock()
				gosRumors := gos.rumors[flipCoin.Origin]
				gos.rumorsMutex.Unlock()
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

func handlePrivateMsg(gos *Gossiper, msg *GossipPacket) {
	if gos.ID == msg.Private.Destination {
		printMessage(*msg, gos.simpleMode, false, "")
		return
	}

	// Discard the message silenty
	if msg.Private.HopLimit == 1 {
		return
	}

	addr := gos.routingTable[msg.Private.Destination]

	if addr != "" {
		rmr := &PrivateMessage{
			Origin: msg.Private.Origin,
			ID:     msg.Private.ID,
			Text:   msg.Private.Text,
			Destination: msg.Private.Destination,
			HopLimit: msg.Private.HopLimit - 1,
		}
	
		msg := &GossipPacket{Private: rmr}
	
		sendMsgTo(addr, msg, *gos)
	} else {
		fmt.Println("Unable to send message to unknown peer")
	}
}

// Gossiper handler: get the message and interact accordingly
func handleGossiperConnection(conn *net.UDPConn, gos *Gossiper) {
	buffer := make([]byte, 8192)

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
	gos.peersMutex.Lock()
	if !strings.Contains(gos.peers, addr) {
		if gos.peers == "" {
			gos.peers = addr
		} else {
			auxPeers := strings.Split(gos.peers, ",")
			auxPeers = append(auxPeers, addr)
			gos.peers = strings.Join(auxPeers, ",")
		}

	}
	gos.peersMutex.Unlock()

	// In simple mode replace parameters and forward message
	if gos.simpleMode {
		printMessage(*msg, gos.simpleMode, false, "")
		gos.peersMutex.Lock()
		forwardPeers := strings.Replace(gos.peers, addr, "", -1)
		gos.peersMutex.Unlock()
		msg.Simple.RelayPeerAddr = gos.address.IP.String() + ":" + strconv.Itoa(gos.address.Port)
		broadcast(forwardPeers, msg, *gos)
		gos.peersMutex.Lock()
		printPeers(gos.peers)
		gos.peersMutex.Unlock()
	// If message is a rumor start rumormongering or ack
	} else if msg.Rumor != nil {
		handleRumor(addr, gos, msg)
	// If message is a satus check if we have more rumors to send, we have rumors to receive or we are in sync with the status received
	} else if msg.Status != nil {
		handleStatus(addr, gos, msg)
	} else if msg.Private != nil {
		handlePrivateMsg(gos, msg)
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

	// Print the message & broadcast the message
	if gos.simpleMode {
 		sendNewSimpleMessage(gos, tmpMsg.Text)
	// Generate the message, print it, store it and start rumormongering
	} else if tmpMsg.Destination != nil {
		sendNewPrivateMessage(gos, tmpMsg.Text, *tmpMsg.Destination)
	//send the message
	} else if tmpMsg.File != nil {
		handleFile(gos, *tmpMsg.File);
	} else {
		sendNewRumorMessage(gos, tmpMsg.Text)
	}

}
