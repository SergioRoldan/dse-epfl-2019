package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"

	"go.dedis.ch/protobuf"
)

/* GLOB VARS */

var timeout int

/* PRINTERS */

// Print an upcoming message wether is simple mode, rumor or status
func printMessage(packedMsg GossipPacket, simpleMode, isClient bool, address string) {
	if simpleMode {
		if isClient {
			//fmt.Println("CLIENT MESSAGE " + packedMsg.Simple.Contents)
		} else {
			//fmt.Println("SIMPLE MESSAGE origin " + packedMsg.Simple.OriginalName + " from " + packedMsg.Simple.RelayPeerAddr + " contents " + packedMsg.Simple.Contents)
		}
	} else if packedMsg.Rumor != nil {
		if isClient {
			fmt.Println("CLIENT MESSAGE " + packedMsg.Rumor.Text)
		} else {
			//fmt.Println("RUMOR origin " + packedMsg.Rumor.Origin + " from " + address + " ID " + fmt.Sprint(packedMsg.Rumor.ID) + " contents " + packedMsg.Rumor.Text)
		}
	} else if packedMsg.Status != nil {
		peers := ""

		for _, status := range packedMsg.Status.Want {
			peers += " peer " + status.Identifier + " nextID " + fmt.Sprint(status.NextID)
		}

		//fmt.Println("STATUS from " + address + peers)
	}
}

func printConfirmedTLC(gos * Gossiper, tlcMessage TLCMessage) {
	// maybe is ID not confirmed
	str := "CONFIRMED GOSSIP origin " + tlcMessage.Origin +
	" ID " + fmt.Sprint(tlcMessage.Confirmed) +
	" file name " + tlcMessage.TxBlock.Transaction.Name +
	" size " + fmt.Sprint(tlcMessage.TxBlock.Transaction.Size) +
	" metahash " + hex.EncodeToString(tlcMessage.TxBlock.Transaction.MetafileHash);
	fmt.Println(str)

	gos.confirmedTLCs = append(gos.confirmedTLCs, str);
	//save the confirmed tlc in an string list
}

func printUnconfirmedTLC(tlcMessage TLCMessage) {
	fmt.Println("UNCONFIRMED GOSSIP origin " + tlcMessage.Origin +
		" ID " + fmt.Sprint(tlcMessage.ID) +
		" file name " + tlcMessage.TxBlock.Transaction.Name +
		" size " + fmt.Sprint(tlcMessage.TxBlock.Transaction.Size) +
		" metahash " + hex.EncodeToString(tlcMessage.TxBlock.Transaction.MetafileHash))
}

func printPrivateMessage(packedMsg PrivateMessage) {
	fmt.Println("PRIVATE origin " + packedMsg.Origin + " hop-limit " + fmt.Sprint(packedMsg.HopLimit) + " contents " + packedMsg.Text)
}

// Print mongering process started with peer
func printRumormongering(address string) {
	//fmt.Println("MONGERING with " + address)
}

// Print routing table update
func printDSDV(origin, addr string) {
	fmt.Println("DSDV " + origin + " " + addr)
}

// Print flipped coin win, seending rumor to peer
func printFlippedCoin(address string) {
	//fmt.Println("FLIPPED COIN sending rumor to " + address)
}

// Print in sync with peer
func printInSync(address string) {
	//fmt.Println("IN SYNC WITH " + address)
}

// Print peers
func printPeers(peers string) {
	//fmt.Println("PEERS " + peers)
}

// Print downloading metafile
func printDownloadingMetafile(name, destination string) {
	fmt.Println("DOWNLOADING metafile of " + name + " from " + destination)
}

// Print downloading chunk
func printDownloadingChunk(name, destination string, chunkCount int) {
	fmt.Println("DOWNLOADING " + name + " chunk " + fmt.Sprint(chunkCount) + " from " + destination)
}

// Print file reconstructed
func printReconstructed(name string) {
	fmt.Println("RECONSTRUCTED file " + name)
}

// Print new private message from our client
func printPrivateClient(text, destination string) {
	fmt.Println("CLIENT MESSAGE " + text + " dest " + destination)
}

func printMatchFound(gos *Gossiper, file, node, hash string, chunks []uint64) {
	var chunkStr []string

	for _, chnk := range chunks {
		chunkStr = append(chunkStr, fmt.Sprint(chnk))
	}

	str := "FOUND match " + file + " at " + node + " metafile=" + hash + " chunks=" + strings.Join(chunkStr, ",")

	fmt.Println(str)

	gos.searchMatches = append(gos.searchMatches, str)
}

func printSearchFinished() {
	fmt.Println("SEARCH FINISHED")
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
	gos.mutexs.statusMutex.Lock()
	stts := &StatusPacket{Want: gos.Status.Want}
	gos.mutexs.statusMutex.Unlock()
	msg := &GossipPacket{Status: stts}
	sendMsgTo(peer, msg, gos)
}

/* STORE MSG */

// Store a message to ack in rumorsToAck list of gossiper
func storeMsgToAck(gos *Gossiper, msg GossipPacket, addr string) {
	found := false

	var origin string
	var id uint32

	if msg.TLCMessage != nil {
		origin = msg.TLCMessage.Origin
		id = msg.TLCMessage.ID
	} else {
		origin = msg.Rumor.Origin
		id = msg.Rumor.ID
	}

	gos.mutexs.rumorsToAckMutex.Lock()
	for _, rumor := range gos.rumorsToAck[addr] {
		if rumor.Origin == origin && rumor.ID == id {
			found = true
			break
		}
	}

	if !found {
		gos.rumorsToAck[addr] = append(gos.rumorsToAck[addr], RumorAck{origin, id})
	}
	gos.mutexs.rumorsToAckMutex.Unlock()
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
	storeMsg(gos, *msg)
	printMessage(*msg, gos.simpleMode, false, addr)
	gos.mutexs.peersMutex.Lock()
	printPeers(gos.peers)
	gos.mutexs.peersMutex.Unlock()
}

// Store a message in gossiper list of rumors
func storeMsg(gos *Gossiper, msg GossipPacket) {
	var origin string

	if msg.TLCMessage != nil {
		origin = msg.TLCMessage.Origin
	} else {
		origin = msg.Rumor.Origin
	}

	gos.mutexs.rumorsMutex.Lock()
	gos.rumors[origin] = append(gos.rumors[origin], msg)
	gos.mutexs.rumorsMutex.Unlock()
}

/* AUXILIAR FUNCTIONS */

// Get a random peer of the peers list of the gossiper
func randPeer(gos Gossiper) string {
	gos.mutexs.peersMutex.Lock()
	defer gos.mutexs.peersMutex.Unlock()

	peersArr := strings.Split(gos.peers, ",")
	return peersArr[rand.Intn(len(peersArr))]
}

/* RUMORMONGERING & ANTIENTROPY */

// Rumormongering process: create a ticker for the ack timeout, create a worker to handle the ack timeout, store the msg to ack, print the rumormongering & send the message
func rumormongering(gos *Gossiper, msg *GossipPacket, addr string) {
	ticker := time.NewTicker(time.Duration(timeout) * time.Second)
	go rumormongeringWorker(gos, msg, addr, ticker)
	storeMsgToAck(gos, *msg, addr)

	printRumormongering(addr)
	sendMsgTo(addr, msg, *gos)
}

// Rumormongering worker in charge of checking if ack is timeout, in that case start rumormongering with a random peer
func rumormongeringWorker(gos *Gossiper, msg *GossipPacket, addr string, ticker *time.Ticker) {

	for {
		select {
		case <-ticker.C:
			found := false

			var origin string
			var id uint32

			if msg.TLCMessage != nil {
				origin = msg.TLCMessage.Origin
				id = msg.TLCMessage.ID
			} else {
				origin = msg.Rumor.Origin
				id = msg.Rumor.ID
			}

			gos.mutexs.rumorsToAckMutex.Lock()
			for j, rumor := range gos.rumorsToAck[addr] {
				if rumor.Origin == origin && rumor.ID == id {
					found = true
					gos.rumorsToAck[addr] = append(gos.rumorsToAck[addr][:j], gos.rumorsToAck[addr][j+1:]...)
					break
				}
			}
			gos.mutexs.rumorsToAckMutex.Unlock()

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
				//fmt.Println("Anti-entropy status send to " + rndPeer)
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
	hw3ex2 := flag.Bool("hw3ex2", false, "gossiper is running in hw3 ex2 mode")
	hw3ex3 := flag.Bool("hw3ex3", false, "gossiper is running in hw3 ex3 mode")
	hw3ex4 := flag.Bool("hw3ex4", false, "gossiper is running in hw3 ex3 mode")
	ackAll := flag.Bool("ackAll", false, "acknowledge every message independently of it ID")
	N := flag.Int("N", 0, "Total number of peers in the network")
	stubbornTimeout := flag.Int("stubbornTimeout", 5, "The stubborn timeout")
	hopLimit := flag.Uint("hopLimit", 10, "private messages hop limit")
	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	// Define mongering timeout
	timeout = *timeoutFlag

	// Create a new gossiper and its worker
	gos := NewGossiper(*gossipAddr, *name, *peers, *simpleMode, *hw3ex2, *hw3ex3, *hw3ex4, *ackAll, *N, *stubbornTimeout, *hopLimit)

	if *hw3ex4 {
		gos.blocks = append(gos.blocks, TLCMessage{})
	}

	if gos != nil {
		go gossipWorker(gos)
	}

	if !*simpleMode {
		// Create a new anti-entropy worker
		tickerA := time.NewTicker(time.Duration(*aeTimer) * time.Second)
		go antientropy(gos, tickerA)
	}

	// If rtimer > 0 start the automatic route rumors thread sender and send a route rumor right away
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
