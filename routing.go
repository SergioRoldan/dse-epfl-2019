package main

import (
	"fmt"
	"time"
)

func updateRoutingEntry(addr, origin string, gos *Gossiper, print bool) {
	gos.routingTable[origin] = addr
	if print {
		printDSDV(origin, addr)
	}

	// DELETE just for debug purposes
	fmt.Println(gos.routingTable)
}

func routeMessage(gos *Gossiper, ticker *time.Ticker) {
	for {
		select {
		case <-ticker.C:
			sendRumorMsg(gos)
		}
	}
}

func sendRumorMsg(gos *Gossiper) {
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
		Text:   "",
	}

	gos.Status.Want[nID].NextID++
	gos.statusMutex.Unlock()

	msg := &GossipPacket{Rumor: rmr}

	storeMsg(gos, *msg.Rumor)

	rndPeer := randPeer(*gos)
	if rndPeer != "" {
		rumormongering(gos, msg, rndPeer)
	}
}