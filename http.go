package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"fmt"
	"strings"
)

const DIR = "./static/"

/* HTTP HANDLERS */
type PrivateHTTPMessage struct {
	Text string
	Destination string
}

// /nodes handler
func nodeHandler(w http.ResponseWriter, r *http.Request, gos *Gossiper) {
	// If GET send the peers in json format
	if r.Method == "GET" {
		gos.peersMutex.Lock()
		peers := NodesResponse{gos.peers}
		gos.peersMutex.Unlock()

		js, err := json.Marshal(peers)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)

		// If POST get the ID and if it's new add it to the list of peers of the gossiper and send 200
	} else if r.Method == "POST" {
		reqBody, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Fatal(err)
		}

		addr := string(reqBody)

		// If peer is new add it to the gossiper's peerster list
		gos.peersMutex.Lock()
		if !strings.Contains(gos.peers, addr) {
			if gos.peers == "" {
				gos.peers = addr
			} else {
				auxPeers := strings.Split(gos.peers, ",")
				auxPeers = append(auxPeers, addr)
				gos.peers = strings.Join(auxPeers, ",")
			}
		}
		gos.peersMutex.Unlock()

		w.WriteHeader(http.StatusOK)
    	w.Write([]byte("200 OK"))

	}
}

// /messages handler
func messageHandler(w http.ResponseWriter, r *http.Request, gos *Gossiper) {
	// If GET send the gossiper messages in json format
	if r.Method == "GET" {
		gos.rumorsMutex.Lock()
		messages := MessagesResponse{ Rumors: gos.rumors }
		//Maybe store it to display it in the gui
		//Privates: gos.privates

		gos.rumorsMutex.Unlock()

		js, err := json.Marshal(messages)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)

		// If POST get the message content, generate a GossiperPacket and start rumormongering or broadcast depending on the mode
	} else if r.Method == "POST" {
		reqBody, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Fatal(err)
		}

		tmpMsg := string(reqBody)

		if gos.simpleMode {
			sendNewSimpleMessage(gos, tmpMsg)
		// Generate the message, print it, store it and start rumormongering
		} else {
			sendNewRumorMessage(gos, tmpMsg)
		}

		w.WriteHeader(http.StatusOK)
    	w.Write([]byte("200 OK"))
	}
}

// /id handler
func idHandler(w http.ResponseWriter, r *http.Request, gos *Gossiper) {
	// If GET send the gossiper id in json format
	if r.Method == "GET" {
		id := IDResponse{gos.ID}

		js, err := json.Marshal(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	} 
}

func usersHandler(w http.ResponseWriter, r *http.Request, gos *Gossiper) {
	// If GET send the peers in json format
	if r.Method == "GET" {
		routingMap := gos.routingTable
		keys := make([]string, 0, len(routingMap))
		for k := range routingMap {
			keys = append(keys, k)
		}
		peers := NodesResponse{strings.Join(keys,",")}

		js, err := json.Marshal(peers)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)

	} else if r.Method == "POST" {
		reqBody, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Fatal(err)
		}

		data := &PrivateHTTPMessage{}

		_ = json.Unmarshal(reqBody, data)
		fmt.Println("reqBody", data.Destination)

		sendNewPrivateMessage(gos, data.Text, data.Destination)

		w.WriteHeader(http.StatusOK)
    	w.Write([]byte("200 OK"))
	}

}

func webserver(gos *Gossiper, UIPort string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/nodes", func(w http.ResponseWriter, r *http.Request) {
		nodeHandler(w, r, gos)
	})
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		messageHandler(w, r, gos)
	})
	mux.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		usersHandler(w, r, gos)
	})
	mux.HandleFunc("/id", func(w http.ResponseWriter, r *http.Request) {
		idHandler(w, r, gos)
	})
	mux.Handle("/", http.FileServer(http.Dir(DIR)))

	log.Printf("Serving %s on HTTP port: %s\n", DIR, UIPort)
	log.Fatal(http.ListenAndServe(":"+UIPort, mux))
}