package main

import (
	"fmt"
	"strconv"
)

func sendNewSimpleMessage(gos *Gossiper, text string) {
	msg := &GossipPacket{}
	msg.Simple = &SimpleMessage{gos.Name, gos.address.IP.String() + ":" + strconv.Itoa(gos.address.Port), text}
	printMessage(*msg, gos.simpleMode, true, "")
	gos.peersMutex.Lock()
	broadcast(gos.peers, msg, *gos)
	gos.peersMutex.Unlock()
}

func sendNewRumorMessage(gos *Gossiper, text string) {
	nID := 0
	gos.statusMutex.Lock()
	for i, n := range gos.Status.Want {
		if n.Identifier == gos.ID {
			nID = i
			break
		}
	}

	rmr := &RumorMessage{
		Origin: gos.ID,
		ID:     gos.Status.Want[nID].NextID,
		Text:   text,
	}

	gos.Status.Want[nID].NextID++
	gos.statusMutex.Unlock()

	msg := &GossipPacket{Rumor: rmr}

	printMessage(*msg, gos.simpleMode, true, "")
	storeMsg(gos, *msg.Rumor)

	rndPeer := randPeer(*gos)
	if rndPeer != "" {
		rumormongering(gos, msg, rndPeer)
	}
}

func sendNewPrivateMessage(gos *Gossiper, text string, destination string) {
	rmr := &PrivateMessage{
		Origin: gos.ID,
		ID:     0,
		Text:   text,
		Destination:	destination,
		HopLimit: 10,
	}

	msg := &GossipPacket{Private: rmr}

	if destination == gos.ID {
		printMessage(*msg, gos.simpleMode, true, "")
		return
	}

	msg.Private.HopLimit -= 1

	addr := gos.routingTable[destination]

	if addr != "" {
		sendMsgTo(addr, msg, *gos)
	} else {
		fmt.Println("Unable to send message to unknown peer")
	}
}