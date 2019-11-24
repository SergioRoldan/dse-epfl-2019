package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"

	"go.dedis.ch/protobuf"
)

// Message structure with Text string
type Message struct {
	Text        string
	Destination *string
	File        *string
	Request     *[]byte
	Keywords    *string
	Budget      *uint64
}

func main() {

	// Parse the command entry
	UIPort := flag.String("UIPort", "8080", "port for the UI Client")
	dest := flag.String("dest", "", "destination for the private message; can be omitted")
	msg := flag.String("msg", "", "message to be sent")
	file := flag.String("file", "", "file to be indexed by the gossiper")
	request := flag.String("request", "", "request a chunk or metafile of this hash")
	keywords := flag.String("keywords", "", "comma separated list of keywords")
	budget := flag.Int("budget", -1, "budget value as integer")
	flag.Parse()

	// Resolve the address and the udp connection
	address := "127.0.0.1:" + *UIPort

	udpAddr, addrErr := net.ResolveUDPAddr("udp4", address)

	if addrErr != nil {
		fmt.Println("Addr Err: " + addrErr.Error())
	}

	udpConn, connErr := net.DialUDP("udp4", nil, udpAddr)

	if connErr != nil {
		fmt.Println("Conn err: " + connErr.Error())
	}

	// Create the msg with the parameters specified in the flags
	newmsg := &Message{Text: *msg}

	if *dest != "" {
		newmsg.Destination = dest
	}

	if *file != "" {
		newmsg.File = file

		if *request != "" {
			req, err := hex.DecodeString(*request)

			if err == nil {
				newmsg.Request = &req
			} else {
				fmt.Println("ERROR (Unable to decode hex hash)")
				os.Exit(1)
			}
		}
	}

	if *keywords != "" {
		tmpKeywords := strings.Split(*keywords, ",")

		var res []string
		for _, keyword := range tmpKeywords {
			if keyword != "" {
				res = append(res, keyword)
			}
		}

		if len(res) > 0 {
			sanKeywords := strings.Join(res, ",")
			newmsg.Keywords = &sanKeywords
			if *budget > 0 {
				sanBudget := uint64(*budget)
				newmsg.Budget = &sanBudget
			}
		}
	}

	// If the flag combination is correct send the message
	if (newmsg.Destination != nil && newmsg.File != nil && newmsg.Request != nil && newmsg.Keywords == nil) ||
		(newmsg.Destination == nil && newmsg.File != nil && newmsg.Request != nil && newmsg.Keywords == nil) ||
		(newmsg.Destination != nil && newmsg.File == nil && newmsg.Request == nil && newmsg.Keywords == nil) ||
		(newmsg.Destination == nil && newmsg.File != nil && newmsg.Request == nil && newmsg.Keywords == nil) ||
		(newmsg.Destination == nil && newmsg.File == nil && newmsg.Request == nil && newmsg.Keywords != nil) ||
		(newmsg.Destination == nil && newmsg.File == nil && newmsg.Request == nil && newmsg.Keywords == nil) {

		packetBytes, encodeErr := protobuf.Encode(newmsg)

		if encodeErr != nil {
			fmt.Println("Encode error: " + encodeErr.Error())
		}

		_, writeErr := udpConn.Write(packetBytes)

		if writeErr != nil {
			fmt.Println("Write error: " + writeErr.Error())
		}
	} else {
		fmt.Println("ERROR (Bad argument combination)")
		os.Exit(1)
	}

}
