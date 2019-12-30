package main

import (
	"fmt"
	"strconv"
)

// Send a new simple message
func sendNewSimpleMessage(gos *Gossiper, text string) {
	msg := &GossipPacket{}
	msg.Simple = &SimpleMessage{gos.Name, gos.address.IP.String() + ":" + strconv.Itoa(gos.address.Port), text}
	printMessage(*msg, gos.simpleMode, true, "")
	gos.mutexs.peersMutex.Lock()
	broadcast(gos.peers, msg, *gos)
	gos.mutexs.peersMutex.Unlock()
}

// Send a new rumor message
func sendNewRumorMessage(gos *Gossiper, text string, tlcMessage *TLCMessage) {
	nID := 0
	gos.mutexs.statusMutex.Lock()
	for i, n := range gos.Status.Want {
		if n.Identifier == gos.ID {
			nID = i
			break
		}
	}

	nextID := gos.Status.Want[nID].NextID

	gos.Status.Want[nID].NextID++
	gos.mutexs.statusMutex.Unlock()

	msg := &GossipPacket{}

	if tlcMessage == nil {
		rmr := &RumorMessage{
			Origin: gos.ID,
			ID:     nextID,
			Text:   text,
		}

		msg.Rumor = rmr

		printMessage(*msg, gos.simpleMode, true, "")
		storeMsg(gos, *msg)
	} else {
		tlcMessage.ID = nextID
		msg.TLCMessage = tlcMessage

		printMessage(*msg, gos.simpleMode, true, "")
		storeMsg(gos, *msg)
	}

	rndPeer := randPeer(*gos)
	if rndPeer != "" {
		rumormongering(gos, msg, rndPeer)
	}
}

// Send new acknowledgement for a TLC Message
func sendNewTLCAck(gos *Gossiper, tlcAck *TLCAck) {
	msg := &GossipPacket{Ack: tlcAck}

	gos.mutexs.routingMutex.Lock()
	addr := gos.routingTable[tlcAck.Destination]
	gos.mutexs.routingMutex.Unlock()

	if addr != "" {
		sendMsgTo(addr, msg, *gos)
	} else {
		fmt.Println("Unable to send message to unknown peer")
	}
}

// Send a new private message
func sendNewPrivateMessage(gos *Gossiper, text string, destination string) {
	rmr := &PrivateMessage{
		Origin:      gos.ID,
		ID:          0,
		Text:        text,
		Destination: destination,
		HopLimit:    HOPLIMIT,
	}

	msg := &GossipPacket{Private: rmr}

	printPrivateClient(text, destination)

	if destination == gos.ID {
		printMessage(*msg, gos.simpleMode, true, "")
		return
	}

	msg.Private.HopLimit--

	gos.mutexs.routingMutex.Lock()
	addr := gos.routingTable[destination]
	gos.mutexs.routingMutex.Unlock()

	if addr != "" {
		sendMsgTo(addr, msg, *gos)
		gos.mutexs.privateMutex.Lock()
		gos.private[msg.Private.Destination] = append(gos.private[msg.Private.Destination], *msg.Private)
		gos.mutexs.privateMutex.Unlock()
	} else {
		fmt.Println("Unable to send message to unknown peer")
	}
}

// Send a new search reply to a search request
func sendNewSearchReply(gos *Gossiper, searchReply *SearchReply) {
	msg := &GossipPacket{SearchReply: searchReply}

	gos.mutexs.routingMutex.Lock()
	addr := gos.routingTable[searchReply.Destination]
	gos.mutexs.routingMutex.Unlock()

	if addr != "" {
		sendMsgTo(addr, msg, *gos)
	} else {
		fmt.Println("Unable to send message to unknown peer")
	}
}

// Forwards a search request
func forwardSearchRequest(gos *Gossiper, origin, addr string, keywords []string, budget uint64) {
	srch := &SearchRequest{
		Origin:   origin,
		Budget:   budget,
		Keywords: keywords,
	}

	msg := &GossipPacket{SearchRequest: srch}

	sendMsgTo(addr, msg, *gos)
}
