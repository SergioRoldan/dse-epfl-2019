package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"
	"sync/atomic"
	"time"
)

// List a new file depending on ex
func listFile(gos *Gossiper, name string, size int64, metafileHash []byte) {

	txPublish := TxPublish{
		Name:         name,
		Size:         size,
		MetafileHash: metafileHash,
	}

	blckPub := BlockPublish{
		Transaction: txPublish,
	}

	if gos.hw3.hw3ex4 {
		gos.mutexs.blocksMutex.Lock()
		bt := gos.blocks[len(gos.blocks)-1]
		gos.mutexs.blocksMutex.Unlock()
		blckPub.PrevHash = bt.TxBlock.Hash()
	}

	tlcMessage := &TLCMessage{
		Origin:    gos.ID,
		Confirmed: -1,
		TxBlock:   blckPub,
		Fitness:   0,
	}

	if gos.hw3.hw3ex3 || gos.hw3.hw3ex4 {
		tlcMessage.VectorClock = &gos.Status
	}

	if gos.hw3.hw3ex4 {
		tlcMessage.Fitness = rand.Float32()
	}

	if gos.hw3.hw3ex3 || gos.hw3.hw3ex4 {
		sendNewGossipWithConf(gos, tlcMessage)
	} else {
		sendNewRumorMessage(gos, "", tlcMessage)
		printUnconfirmedTLC(*tlcMessage)

		gos.mutexs.tlcMessagesMutex.Lock()
		gos.tlcMessages[tlcMessage.Origin] = append(gos.tlcMessages[tlcMessage.Origin], *tlcMessage)
		gos.mutexs.tlcMessagesMutex.Unlock()

		ch := make(chan TLCAck, 10)
		gos.mutexs.tlcAcksMutex.Lock()
		gos.tlcAcks = append(gos.tlcAcks, ch)
		gos.mutexs.tlcAcksMutex.Unlock()

		go waitForAckTLC(gos, *tlcMessage, ch)
	}
}

// Wait for the ackowledgement of a TLC Message
func waitForAckTLC(gos *Gossiper, tlcMessage TLCMessage, ch chan TLCAck) {
	acks := make(map[string]bool)

	acks[tlcMessage.Origin] = true

	ticker := time.NewTicker(time.Duration(gos.hw3.stubbornTout) * time.Second)

	for {
		select {
		case tlcAck, ok := <-ch:

			if !ok {
				deleteTLCAcksChannel(gos, ch)
				return
			}

			if tlcAck.ID == tlcMessage.ID {
				if !acks[tlcAck.Origin] {
					acks[tlcAck.Origin] = true

					if len(acks) > int(gos.hw3.N/2) {
						ticker.Stop()

						gos.mutexs.tlcMessagesMutex.Lock()
						for j, tlcM := range gos.tlcMessages[tlcMessage.Origin] {
							if tlcM.ID == tlcMessage.ID {
								gos.tlcMessages[tlcMessage.Origin][j].Confirmed = int(tlcMessage.ID)
								break
							}
						}
						gos.mutexs.tlcMessagesMutex.Unlock()

						confirmedTLC := tlcMessage

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

						confirmedTLC.ID = nextID
						confirmedTLC.Confirmed = int(tlcMessage.ID)

						msg := &GossipPacket{}
						msg.TLCMessage = &confirmedTLC

						if gos.hw3.hw3ex3 || gos.hw3.hw3ex4 {
							msg.TLCMessage.VectorClock = &gos.Status
						}

						printMessage(*msg, gos.simpleMode, true, "")
						storeMsg(gos, *msg)

						keys := make([]string, 0, len(acks))
						for k := range acks {
							keys = append(keys, k)
						}

						str := "RE-BROADCAST ID " + fmt.Sprint(tlcMessage.ID) + " WITNESSES " + strings.Join(keys, ",")
						fmt.Println(str)
						gos.confirmedTLCs = append(gos.confirmedTLCs, str)

						rndPeer := randPeer(*gos)
						if rndPeer != "" {
							rumormongering(gos, msg, rndPeer)
						}

						if !gos.hw3.hw3ex2 {
							incrementMytime(gos)
						}

						deleteTLCAcksChannel(gos, ch)

						return
					}
				}
			}

		case <-ticker.C:
			resendTLC := tlcMessage

			msg := &GossipPacket{}
			msg.TLCMessage = &resendTLC

			printUnconfirmedTLC(resendTLC)

			rndPeer := randPeer(*gos)
			if rndPeer != "" {
				rumormongering(gos, msg, rndPeer)
			}
		}
	}
}

// Increment myTime and advance round and/or consensus round
func incrementMytime(gos *Gossiper) {
	sum := 0
	var witnesses []string

	gos.mutexs.tlcMessagesMutex.Lock()
	for _, v := range gos.tlcMessages {
		for _, tlc := range v {
			if len(gos.tlcMessages[gos.ID]) > 0 && tlc.Confirmed == int(gos.tlcMessages[gos.ID][len(gos.tlcMessages[gos.ID])-1].ID) {
				sum++
				witnesses = append(witnesses, "origin"+fmt.Sprint(sum)+" "+tlc.Origin+" ID"+fmt.Sprint(sum)+" "+fmt.Sprint(tlc.Confirmed))
				break
			}
		}
	}
	gos.mutexs.tlcMessagesMutex.Unlock()

	gos.mutexs.gossipWithConfMutex.Lock()
	lenGossipConf := len(gos.gossipWithConf) - 1
	gos.mutexs.gossipWithConfMutex.Unlock()

	if sum > int(gos.hw3.N/2) && lenGossipConf >= int(atomic.LoadUint32(gos.myTime)) {
		atomic.AddUint32(gos.myTime, 1)
		gos.mutexs.tlcAcksMutex.Lock()
		for _, ch := range gos.tlcAcks {
			close(ch)
		}
		gos.mutexs.tlcAcksMutex.Unlock()

		adv := "ADVANCING TO round " + fmt.Sprint(atomic.LoadUint32(gos.myTime)) + " BASED ON CONFIRMED MESSAGES " + strings.Join(witnesses, ", ")
		gos.roundAdvances = append(gos.roundAdvances, adv)
		fmt.Println(adv)

		if gos.hw3.hw3ex4 && atomic.LoadUint32(gos.myTime) > 0 && atomic.LoadUint32(gos.myTime)%3 == 0 {
			fittest := findFittestTLC(gos)

			var fileNames []string

			gos.mutexs.blocksMutex.Lock()
			gos.blocks = append(gos.blocks, fittest)

			for _, block := range gos.blocks {
				fileNames = append(fileNames, block.TxBlock.Transaction.Name)
			}
			gos.mutexs.blocksMutex.Unlock()

			str := "CONSENSUS ON QSC round " + fmt.Sprint(atomic.LoadUint32(gos.myTime)/3-1) +
				" message origin " + fittest.Origin + " ID " + fmt.Sprint(atomic.LoadUint32(gos.myTime)-3) +
				" file names " + strings.Join(fileNames, " ") + " size " +
				fmt.Sprint(fittest.TxBlock.Transaction.Size) + " metahash " +
				hex.EncodeToString(fittest.TxBlock.Transaction.MetafileHash)

			fmt.Println(str)
			gos.consensusOn = append(gos.consensusOn, str)

		} else if gos.hw3.hw3ex4 && atomic.LoadUint32(gos.myTime)%3 != 0 {
			fittest := findFittestTLC(gos)

			if fittest.Origin != "" {
				hash := fittest.TxBlock.Hash()
				fittest.Origin = gos.ID
				fittest.Confirmed = -1
				fittest.VectorClock = &gos.Status
				fittest.TxBlock.PrevHash = hash
				fittest.VectorClock = &gos.Status

				gos.mutexs.tlcMessagesMutex.Lock()
				gos.tlcMessages[fittest.Origin] = append(gos.tlcMessages[fittest.Origin], fittest)
				gos.mutexs.tlcMessagesMutex.Unlock()
				printUnconfirmedTLC(fittest)

				sendNewRumorMessage(gos, "", &fittest)

				ch := make(chan TLCAck, 10)
				gos.mutexs.tlcAcksMutex.Lock()
				gos.tlcAcks = append(gos.tlcAcks, ch)
				gos.mutexs.tlcAcksMutex.Unlock()

				gos.mutexs.gossipWithConfMutex.Lock()
				gos.gossipWithConf = append(gos.gossipWithConf, gossipWithConfChannel{round: atomic.LoadUint32(gos.myTime)})
				gos.mutexs.gossipWithConfMutex.Unlock()

				go waitForAckTLC(gos, fittest, ch)
			}
		}

		if gos.hw3.hw3ex3 || (gos.hw3.hw3ex4 && atomic.LoadUint32(gos.myTime)%3 == 0) {
			gos.mutexs.gossipWithConfMutex.Lock()
			for _, chn := range gos.gossipWithConf {
				if chn.ch != nil {
					if !gos.hw3.hw3ex4 {
						chn.ch <- int(atomic.LoadUint32(gos.myTime))
					} else {
						chn.ch <- int(atomic.LoadUint32(gos.myTime) / 3)
					}
				}
			}
			gos.mutexs.gossipWithConfMutex.Unlock()
		}
	}
}

// Send TLC message with confirmation, aka with waiting time
func sendNewGossipWithConf(gos *Gossiper, tlcMessage *TLCMessage) {
	gos.mutexs.gossipWithConfMutex.Lock()
	lenGossipConf := len(gos.gossipWithConf)
	gos.mutexs.gossipWithConfMutex.Unlock()

	if !gos.hw3.hw3ex4 {
		if lenGossipConf == int(atomic.LoadUint32(gos.myTime)) {
			sendNewRumorMessage(gos, "", tlcMessage)
			printUnconfirmedTLC(*tlcMessage)
			gos.mutexs.tlcMessagesMutex.Lock()
			gos.tlcMessages[tlcMessage.Origin] = append(gos.tlcMessages[tlcMessage.Origin], *tlcMessage)
			gos.mutexs.tlcMessagesMutex.Unlock()

			gos.mutexs.gossipWithConfMutex.Lock()
			gos.gossipWithConf = append(gos.gossipWithConf, gossipWithConfChannel{round: atomic.LoadUint32(gos.myTime)})
			gos.mutexs.gossipWithConfMutex.Unlock()

			ch := make(chan TLCAck, 10)
			gos.mutexs.tlcAcksMutex.Lock()
			gos.tlcAcks = append(gos.tlcAcks, ch)
			gos.mutexs.tlcAcksMutex.Unlock()

			go waitForAckTLC(gos, *tlcMessage, ch)
			incrementMytime(gos)
		} else {
			ch := make(chan int)
			gos.mutexs.gossipWithConfMutex.Lock()
			gosChan := gossipWithConfChannel{round: uint32(len(gos.gossipWithConf)), ch: ch}
			gos.gossipWithConf = append(gos.gossipWithConf, gosChan)
			gos.mutexs.gossipWithConfMutex.Unlock()
			go waitForRound(gos, tlcMessage, gosChan)
		}
	} else {
		if (lenGossipConf == int(atomic.LoadUint32(gos.myTime)/7) && atomic.LoadUint32(gos.myTime)%7 == 0) || lenGossipConf == 0 {
			sendNewRumorMessage(gos, "", tlcMessage)
			printUnconfirmedTLC(*tlcMessage)
			gos.mutexs.tlcMessagesMutex.Lock()
			gos.tlcMessages[tlcMessage.Origin] = append(gos.tlcMessages[tlcMessage.Origin], *tlcMessage)
			gos.mutexs.tlcMessagesMutex.Unlock()

			gos.mutexs.gossipWithConfMutex.Lock()
			gos.gossipWithConf = append(gos.gossipWithConf, gossipWithConfChannel{round: atomic.LoadUint32(gos.myTime)})
			gos.mutexs.gossipWithConfMutex.Unlock()

			ch := make(chan TLCAck, 10)
			gos.mutexs.tlcAcksMutex.Lock()
			gos.tlcAcks = append(gos.tlcAcks, ch)
			gos.mutexs.tlcAcksMutex.Unlock()

			go waitForAckTLC(gos, *tlcMessage, ch)
			incrementMytime(gos)
		} else {
			ch := make(chan int)
			gos.mutexs.gossipWithConfMutex.Lock()
			gosChan := gossipWithConfChannel{round: uint32(len(gos.gossipWithConf)), ch: ch}
			gos.gossipWithConf = append(gos.gossipWithConf, gosChan)
			gos.mutexs.gossipWithConfMutex.Unlock()
			go waitForRound(gos, tlcMessage, gosChan)
		}
	}

	return
}

// Wait for a new round if the message has been halted
func waitForRound(gos *Gossiper, tlcMessage *TLCMessage, gosChan gossipWithConfChannel) {
	for {
		select {
		case round := <-gosChan.ch:
			if round == int(gosChan.round) {
				sendNewRumorMessage(gos, "", tlcMessage)
				printUnconfirmedTLC(*tlcMessage)

				gos.mutexs.tlcMessagesMutex.Lock()
				gos.tlcMessages[tlcMessage.Origin] = append(gos.tlcMessages[tlcMessage.Origin], *tlcMessage)
				gos.mutexs.tlcMessagesMutex.Unlock()

				ch := make(chan TLCAck, 10)
				gos.mutexs.tlcAcksMutex.Lock()
				gos.tlcAcks = append(gos.tlcAcks, ch)
				gos.mutexs.tlcAcksMutex.Unlock()

				go waitForAckTLC(gos, *tlcMessage, ch)

				i := -1

				gos.mutexs.gossipWithConfMutex.Lock()
				defer gos.mutexs.gossipWithConfMutex.Unlock()

				for k, v := range gos.gossipWithConf {
					if v.ch == gosChan.ch {
						i = k
					}
				}

				if i != -1 {
					gos.gossipWithConf = append(gos.gossipWithConf[:i], gos.gossipWithConf[i+1:]...)
				}

				return
			}
		}
	}
}

// Acknowledge a TLC if its not confirmed or its ours
func ackTLC(gos *Gossiper, tlcMessage TLCMessage) {

	gos.mutexs.tlcMessagesMutex.Lock()
	fnd := false
	for _, tlcM := range gos.tlcMessages[tlcMessage.Origin] {
		if tlcM.Confirmed != -1 && int(tlcMessage.ID) == tlcM.Confirmed {
			fnd = true
		}
	}
	gos.mutexs.tlcMessagesMutex.Unlock()

	if fnd || tlcMessage.Origin == gos.ID {
		return
	}

	fmt.Println("SENDING ACK origin " + tlcMessage.Origin + " ID " + fmt.Sprint(tlcMessage.ID))

	tlcAck := &TLCAck{
		Origin:      gos.ID,
		ID:          tlcMessage.ID,
		Text:        "",
		Destination: tlcMessage.Origin,
		HopLimit:    gos.hw3.hopLimit - 1,
	}

	sendNewTLCAck(gos, tlcAck)
}

// Store a TLC out of order
func storeOutOfOrderTLC(gos *Gossiper, tlcMessage TLCMessage) {
	fnd := false

	for _, val := range gos.unconfirmedTLC[tlcMessage.Origin] {
		if val.Confirmed == tlcMessage.Confirmed {
			fnd = true
			break
		}
	}

	if !fnd {
		gos.unconfirmedTLC[tlcMessage.Origin] = append(gos.unconfirmedTLC[tlcMessage.Origin], tlcMessage)
	}
}

// Confirm a TLC out of order
func confirmOutofOrderTLC(gos *Gossiper, tlcMessage TLCMessage) {
	for j, val := range gos.unconfirmedTLC[tlcMessage.Origin] {
		if val.Confirmed == int(tlcMessage.ID) {
			gos.unconfirmedTLC[tlcMessage.Origin] = append(gos.unconfirmedTLC[tlcMessage.Origin][:j], gos.unconfirmedTLC[tlcMessage.Origin][j+1:]...)
			printConfirmedTLC(gos, val)
			incrementMytime(gos)
			break
		}
	}
}

// Auxiliar functions

func (b *BlockPublish) Hash() (out [32]byte) {
	h := sha256.New()
	h.Write(b.PrevHash[:])
	th := b.Transaction.Hash()
	h.Write(th[:])
	copy(out[:], h.Sum(nil))
	return
}

func (t *TxPublish) Hash() (out [32]byte) {
	h := sha256.New()
	binary.Write(h, binary.LittleEndian,
		uint32(len(t.Name)))
	h.Write([]byte(t.Name))
	h.Write(t.MetafileHash)
	copy(out[:], h.Sum(nil))
	return
}

// Validate a TLCMessage depending on the history, ie the previous hash
func validBlockPublish(gos *Gossiper, tlcMessage TLCMessage) bool {
	gos.mutexs.blocksMutex.Lock()
	defer gos.mutexs.blocksMutex.Unlock()

	for _, block := range gos.blocks {
		if block.TxBlock.Transaction.Name == tlcMessage.TxBlock.Transaction.Name {
			return false
		}
	}

	return tlcMessage.TxBlock.PrevHash == gos.blocks[len(gos.blocks)-1].TxBlock.Hash() || *gos.myTime%3 != 0
}

// Delete a channel from the gossiper list to get it garbage collect it
func deleteTLCAcksChannel(gos *Gossiper, ch chan TLCAck) {
	i := -1

	gos.mutexs.tlcAcksMutex.Lock()
	for k, v := range gos.tlcAcks {
		if v == ch {
			i = k
		}
	}

	if i != -1 {
		gos.tlcAcks = append(gos.tlcAcks[:i], gos.tlcAcks[i+1:]...)
	}
	gos.mutexs.tlcAcksMutex.Unlock()

	return
}

// Find the fittest TLC in a TLC round
func findFittestTLC(gos *Gossiper) TLCMessage {
	fitness := float32(0)
	var fittest TLCMessage

	gos.mutexs.tlcMessagesMutex.Lock()
	for k := range gos.tlcMessages {
		for _, tlcMessage := range gos.tlcMessages[k] {
			if tlcMessage.Fitness >= fitness && tlcMessage.Confirmed == int(gos.tlcMessages[gos.ID][len(gos.tlcMessages[gos.ID])-1].ID) {
				fitness = tlcMessage.Fitness
				fittest = tlcMessage
			}
		}
	}
	gos.mutexs.tlcMessagesMutex.Unlock()

	return fittest
}
