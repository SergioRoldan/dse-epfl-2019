package main

import (
	"time"
)

// Update a routing entry and print a DSDV message if the table will be updated
func updateRoutingEntry(addr, origin string, gos *Gossiper, print bool) {
	gos.mutexs.routingMutex.Lock()
	defer gos.mutexs.routingMutex.Unlock()
	diff := gos.routingTable[origin] != addr

	if diff {
		gos.routingTable[origin] = addr
		if print {
			printDSDV(origin, addr)
		}
	}
}

// Send a route rumor when ticker ticks
func routeMessage(gos *Gossiper, ticker *time.Ticker) {
	for {
		select {
		case <-ticker.C:
			sendRumorMsg(gos)
		}
	}
}

// Send a new route rumor message
func sendRumorMsg(gos *Gossiper) {
	nID := 0
	gos.mutexs.statusMutex.Lock()
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
	gos.mutexs.statusMutex.Unlock()

	msg := &GossipPacket{Rumor: rmr}

	storeMsg(gos, *msg)

	rndPeer := randPeer(*gos)
	if rndPeer != "" {
		rumormongering(gos, msg, rndPeer)
	}
}
