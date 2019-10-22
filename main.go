package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net"
	"time"
	"strings"

	"go.dedis.ch/protobuf"
)

/* GLOB VARS */

var timeout int

/* PRINTERS */

// Print an upcoming message wether is simple mode, rumor or status
func printMessage(packedMsg GossipPacket, simpleMode, isClient bool, address string) {
	if simpleMode {
		if isClient {
			fmt.Println("CLIENT MESSAGE " + packedMsg.Simple.Contents)
		} else {
			fmt.Println("SIMPLE MESSAGE origin " + packedMsg.Simple.OriginalName + " from " + packedMsg.Simple.RelayPeerAddr + " contents " + packedMsg.Simple.Contents)
		}
	} else if packedMsg.Rumor != nil {
		if isClient {
			fmt.Println("CLIENT MESSAGE " + packedMsg.Rumor.Text)
		} else {
			fmt.Println("RUMOR origin " + packedMsg.Rumor.Origin + " from " + address + " ID " + fmt.Sprint(packedMsg.Rumor.ID) + " contents " + packedMsg.Rumor.Text)
		}
	} else if packedMsg.Status != nil {
		peers := ""

		for _, status := range packedMsg.Status.Want {
			peers += " peer " + status.Identifier + " nextID " + fmt.Sprint(status.NextID)
		}

		fmt.Println("STATUS from " + address + peers)
	} else if packedMsg.Private != nil {
		fmt.Println("PRIVATE origin " + packedMsg.Private.Origin + " hop-limit " + fmt.Sprint(packedMsg.Private.HopLimit) + " contents " + packedMsg.Private.Text)
	}
}

// Print mongering process started with peer
func printRumormongering(address string) {
	fmt.Println("MONGERING with " + address)
}

func printDSDV(origin, addr string) {
	fmt.Println("DSDV " + origin + " " + addr)
}

// Print flipped coin win, seending rumor to peer
func printFlippedCoin(address string) {
	fmt.Println("FLIPPED COIN sending rumor to " + address)
}

// Print in sync with peer
func printInSync(address string) {
	fmt.Println("IN SYNC WITH " + address)
}

// Print peers
func printPeers(peers string) {
	fmt.Println("PEERS " + peers)
}

/* SEND MESSAGES */

// Simple mode broadcast
func broadcast(peers string, msg *GossipPacket, gos Gossiper) {

	if peers == "" {
		return
	}

	// Encode message, resolve addres and send a message for each of the gossiper peers
	for _, peer := range strings.Split(peers, ",") {
		if peer == "" {
			continue
		}

		packetBytes, encodeErr := protobuf.Encode(msg)

		if encodeErr != nil {
			fmt.Println("Encode err: " + encodeErr.Error())
		}

		udpAddr, addrErr := net.ResolveUDPAddr("udp4", peer)

		if addrErr != nil {
			fmt.Println("Addr Err: " + addrErr.Error())
		}

		_, writeErr := gos.conn.WriteToUDP(packetBytes, udpAddr)

		if writeErr != nil {
			fmt.Println("Send Err: " + writeErr.Error())
		}
	}
}

// Send a GossipPacket message to a peer
func sendMsgTo(peer string, msg *GossipPacket, gos Gossiper) {
	packetBytes, encodeErr := protobuf.Encode(msg)

	if encodeErr != nil {
		fmt.Println("Encode err: " + encodeErr.Error())
	}

	udpAddr, addrErr := net.ResolveUDPAddr("udp4", peer)

	if addrErr != nil {
		fmt.Println("Addr Err: " + addrErr.Error())
	}

	_, writeErr := gos.conn.WriteToUDP(packetBytes, udpAddr)

	if writeErr != nil {
		fmt.Println("Send Err: " + writeErr.Error())
	}

}

// Send status to a peer
func sendStatusTo(peer string, gos Gossiper) {
	gos.statusMutex.Lock()
	stts := &StatusPacket{Want: gos.Status.Want}
	gos.statusMutex.Unlock()
	msg := &GossipPacket{Status: stts}
	sendMsgTo(peer, msg, gos)
}

/* STORE MSG */

// Store a message to ack in rumorsToAck list of gossiper
func storeMsgToAck(gos *Gossiper, msg RumorMessage, addr string) {
	found := false

	gos.rumorsToAckMutex.Lock()
	for _, rumor := range gos.rumorsToAck[addr] {
		if rumor.Origin == msg.Origin && rumor.ID == msg.ID {
			found = true
			break
		}
	}

	if !found {
		gos.rumorsToAck[addr] = append(gos.rumorsToAck[addr], RumorAck{msg.Origin, msg.ID})
	}
	gos.rumorsToAckMutex.Unlock()
}

// Store an new acked message that has not flipped a coin yet
func storeAckedMessage(gos *Gossiper, msgs []RumorAck, addr string) {

	for _, rumor := range msgs {
		found := false
		for _, msg := range gos.rumorsAcked[addr] {
			if msg.Origin == rumor.Origin && msg.ID == rumor.ID {
				found = true
			}
		}

		if !found {
			gos.rumorsAcked[addr] = append(gos.rumorsAcked[addr], RumorAck{rumor.Origin, rumor.ID})
		}
	}

}

// Store a new message in gossiper and print message and peers
func storeNewMsg(gos *Gossiper, msg *GossipPacket, addr string) {
	storeMsg(gos, *msg.Rumor)
	printMessage(*msg, gos.simpleMode, false, addr)
	gos.peersMutex.Lock()
	printPeers(gos.peers)
	gos.peersMutex.Unlock()
}

// Store a message in gossiper list of rumors
func storeMsg(gos *Gossiper, msg RumorMessage) {
	gos.rumorsMutex.Lock()
	gos.rumors[msg.Origin] = append(gos.rumors[msg.Origin], msg)
	gos.rumorsMutex.Unlock()
}

/* AUXILIAR FUNCTIONS */

// Get a random peer of the peers list of the gossiper
func randPeer(gos Gossiper) string {
	gos.peersMutex.Lock()
	defer gos.peersMutex.Unlock()

	peersArr := strings.Split(gos.peers, ",")
	return peersArr[rand.Intn(len(peersArr))]
}

/* RUMORMONGERING & ANTIENTROPY */

// Rumormongering process: create a ticker for the ack timeout, create a worker to handle the ack timeout, store the msg to ack, print the rumormongering & send the message
func rumormongering(gos *Gossiper, msg *GossipPacket, addr string) {
	ticker := time.NewTicker(time.Duration(timeout) * time.Second)
	go rumormongeringWorker(gos, msg, addr, ticker)
	storeMsgToAck(gos, *msg.Rumor, addr)

	printRumormongering(addr)
	sendMsgTo(addr, msg, *gos)
}

// Rumormongering worker in charge of checking if ack is timeout, in that case start rumormongering with a random peer
func rumormongeringWorker(gos *Gossiper, msg *GossipPacket, addr string, ticker *time.Ticker) {

	for {
		select {
		case <-ticker.C:
			found := false

			gos.rumorsToAckMutex.Lock()
			for j, rumor := range gos.rumorsToAck[addr] {
				if rumor.Origin == msg.Rumor.Origin && rumor.ID == msg.Rumor.ID {
					found = true
					gos.rumorsToAck[addr] = append(gos.rumorsToAck[addr][:j], gos.rumorsToAck[addr][j+1:]...)
					break
				}
			}
			gos.rumorsToAckMutex.Unlock()

			ticker.Stop()

			if found {
				rndPeer := randPeer(*gos)
				if rndPeer != "" {
					rumormongering(gos, msg, rndPeer)
				}
			}

			return
		}
	}
}

// Anti-entropy process: when ticker ticks send status to random peer
func antientropy(gos *Gossiper, ticker *time.Ticker) {
	for {
		select {
		case <-ticker.C:
			rndPeer := randPeer(*gos)
			if rndPeer != "" {
				sendStatusTo(rndPeer, *gos)
				fmt.Println("Anti-entropy status send to " + rndPeer)
			}
		}
	}
}

func main() {

	// Parse the command input
	UIPort := flag.String("UIPort", "8080", "port for the UI Client")
	gossipAddr := flag.String("gossipAddr", "127.0.0.1:5000", "ip:port for the gossiper")
	name := flag.String("name", "312348", "name of the gossiper")
	peers := flag.String("peers", "", "comma separated list of peers of the form ip:port")
	simpleMode := flag.Bool("simple", false, "run the gossiper in simple broadcast mode")
	aeTimer := flag.Int("antiEntropy", 10, "timeout in seconds for anti-entropy. If the flag is absent, the default anti-entropy duration is 10 seconds.")
	webclient := flag.Bool("webclient", false, "starts a web server to handle the request of a web client in the port UIPort")
	timeoutFlag := flag.Int("rumongTimer", 10, "rumormongering timeout in seconds")
	rtimer := flag.Int("rtimer", 0, "timeout in seconds to send route rumors. 0 (default) means disable sending route rumors.")
	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	// Define mongering timeout
	timeout = *timeoutFlag

	// Create a new gossiper and its worker
	gos := NewGossiper(*gossipAddr, *name, *peers, *simpleMode)
	if gos != nil {
		go gossipWorker(gos)
	}

	if !*simpleMode {
		// Create a new anti-entropy worker
		tickerA := time.NewTicker(time.Duration(*aeTimer) * time.Second)
		go antientropy(gos, tickerA)
	}

	if *rtimer > 0 {
		sendRumorMsg(gos)
		tickerR := time.NewTicker(time.Duration(*rtimer) * time.Second)
		go routeMessage(gos, tickerR)
	}

	// If webclient start web server for the client
	if *webclient {
		webserver(gos, *UIPort)
	// Otherwise open a UDP connection for the client
	} else {
		address := "127.0.0.1:" + *UIPort

		udpAddr, addrErr := net.ResolveUDPAddr("udp4", address)

		if addrErr != nil {
			fmt.Println("Addr Err: " + addrErr.Error())
		}

		udpConn, connErr := net.ListenUDP("udp4", udpAddr)

		if connErr != nil {
			fmt.Println("Conn err: " + connErr.Error())
			return
		}

		fmt.Println("Server listening in port: " + address)

		defer udpConn.Close()

		for {
			handleClientConnection(udpConn, gos)
		}
	}

}
