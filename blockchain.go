package main

import (
	"fmt"
	"time"
)

func listFile(gos *Gossiper, name string, size int64, metafileHash []byte) {

	txPublish := TxPublish{
		Name:         name,
		Size:         size,
		MetafileHash: metafileHash,
	}

	blckPub := BlockPublish{
		Transaction: txPublish,
	}

	tlcMessage := &TLCMessage{
		Origin:    gos.ID,
		Confirmed: false,
		TxBlock:   blckPub,
		Fitness:   0,
	}

	sendNewRumorMessage(gos, "", tlcMessage)

	ch := make(chan TLCAck)
	gos.tlcAcks = append(gos.tlcAcks, ch)

	go waitForAckTLC(gos, *tlcMessage, ch)
}

func waitForAckTLC(gos *Gossiper, tlcMessage TLCMessage, ch chan TLCAck) {
	acks := make(map[string]bool)

	acks[tlcMessage.Origin] = true

	ticker := time.NewTicker(time.Duration(gos.stubbornTout) * time.Second)

	for {
		select {
		case tlcAck := <-ch:
			fmt.Println(tlcAck)

			if tlcAck.ID == tlcMessage.ID {
				if !acks[tlcAck.Origin] {
					acks[tlcAck.Origin] = true

					if len(acks) > int(gos.N/2) {
						ticker.Stop()

						confirmedTLC := tlcMessage
						confirmedTLC.Confirmed = true
						sendNewRumorMessage(gos, "", &confirmedTLC)

						i := -1

						for k, v := range gos.tlcAcks {
							if v == ch {
								i = k
							}
						}

						if i != -1 {
							gos.tlcAcks = append(gos.tlcAcks[:i], gos.tlcAcks[i+1:]...)
						}
						return
					}
				}
			}

		case <-ticker.C:
			resendTLC := tlcMessage
			sendNewRumorMessage(gos, "", &resendTLC)
		}
	}
}

func ackTLC(gos *Gossiper, tlcMessage TLCMessage) {

	tlcAck := &TLCAck{
		Origin:      gos.ID,
		ID:          tlcMessage.ID,
		Text:        "",
		Destination: tlcMessage.Origin,
		HopLimit:    gos.hopLimit - 1,
	}

	sendNewTLCAck(gos, tlcAck)
}
