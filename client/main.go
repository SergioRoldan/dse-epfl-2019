package main

import (
	"flag"
	"fmt"
	"net"

	"go.dedis.ch/protobuf"
)

// Message structure with Text string
type Message struct {
	Text string
	Destination *string
	File *string
	Request *[]byte
}

func main() {

	// Parse the command entry
	UIPort := flag.String("UIPort", "8080", "port for the UI Client")
	dest := flag.String("dest", "", "destination for the private message; can be omitted")
	msg := flag.String("msg", "", "message to be sent")
	file := flag.String("file", "", "file to be indexed by the gossiper")
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

	// Print parameters
	fmt.Println("Local UDP client address : " + udpConn.LocalAddr().String())
	fmt.Println("Message to be send: " + *msg)
	fmt.Println("Established connection to " + address)

	newmsg := &Message{ Text: *msg }
	if *dest != "" {
		newmsg.Destination = dest
	} else if *file != "" {
		newmsg.File = file
	}

	packetBytes, encodeErr := protobuf.Encode(newmsg)

	if encodeErr != nil {
		fmt.Println("Encode error: " + encodeErr.Error())
	}

	_, writeErr := udpConn.Write(packetBytes)

	if writeErr != nil {
		fmt.Println("Write error: " + writeErr.Error())
	}

}
