package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"fmt"
	"strings"
	"os"
	"encoding/hex"
)

// Constants
const DIR = "./static/"

// PrivateHTTPMessage structure with Text and Destination strings
type PrivateHTTPMessage struct {
	Text string
	Destination string
}

// DownloadHTTPRequest structure with Name, Hash and Peer strings
type DownloadHTTPRequest struct {
	Name string
	Hash string
	Peer string
}

/* HTTP HANDLERS */

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

// messages handler
func messageHandler(w http.ResponseWriter, r *http.Request, gos *Gossiper) {
	// If GET send the gossiper messages in json format
	if r.Method == "GET" {
		gos.rumorsMutex.Lock()
		messages := MessagesResponse{ gos.rumors }

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

// private handler
func privateHandler(w http.ResponseWriter, r *http.Request, gos *Gossiper) {
	// If GET send the private messages with user node in json format
	if r.Method == "GET" {
		user := r.URL.Query().Get("user")

		if len(user) == 0 {
			w.WriteHeader(http.StatusBadRequest)
    		w.Write([]byte("Not users specified"))
			return
		}

		private := PrivateResponse{ gos.private[user] }

		js, err := json.Marshal(private)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	} 
}

// upload handler
func uploadHandler(w http.ResponseWriter, r *http.Request, gos *Gossiper) {
	// If POST write the file in _SharedFiles folder and index it
	if r.Method == "POST" {
		file, handler, err := r.FormFile("upload")
		if err != nil {
			fmt.Println("Form error: " + err.Error())
		}

		f, err := os.Create(UPDIR + handler.Filename)
		if err != nil {
			fmt.Println("File error: " + err.Error())
		}

		fileBytes, err := ioutil.ReadAll(file)
		if err != nil {
			fmt.Println("Read error: " + err.Error())
		}

		f.Write(fileBytes)
		f.Close()

		handleFileIndexing(gos, handler.Filename)

		w.WriteHeader(http.StatusOK)
    	w.Write([]byte("200 OK"))
	} 
}

// download handler
func downloadHandler(w http.ResponseWriter, r *http.Request, gos *Gossiper) {
	// If POST start file download similarly to gossiper's HandleClientConnection for download type messages
	if r.Method == "POST" {
		reqBody, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Fatal(err)
		}

		data := &DownloadHTTPRequest{}

		_ = json.Unmarshal(reqBody, data)

		hash, err := hex.DecodeString(data.Hash)

		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
    		w.Write([]byte("ERROR (Unable to decode hex hash)"))
		} 

		tmpMsg := Message {
			Text: "",
			Destination: &data.Peer,
			File: &data.Name,
			Request: &hash,
		}

		ch := make(chan DataReply)
		gos.downloads = append(gos.downloads, ch)
		go handleFileDownload(gos, tmpMsg, ch)

		w.WriteHeader(http.StatusOK)
    	w.Write([]byte("200 OK"))
	}
}

// users handler
func usersHandler(w http.ResponseWriter, r *http.Request, gos *Gossiper) {
	// If GET send the nodes in json format
	if r.Method == "GET" {
		gos.routingMutex.Lock()
		routingMap := gos.routingTable
		gos.routingMutex.Unlock()
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
	// If POST send a new private message to that user
	} else if r.Method == "POST" {
		reqBody, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Fatal(err)
		}

		data := &PrivateHTTPMessage{}

		_ = json.Unmarshal(reqBody, data)

		sendNewPrivateMessage(gos, data.Text, data.Destination)

		w.WriteHeader(http.StatusOK)
    	w.Write([]byte("200 OK"))
	}

}

// Web server handlers and startup
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
	mux.HandleFunc("/private", func(w http.ResponseWriter, r *http.Request) {
		privateHandler(w, r, gos)
	})
	mux.HandleFunc("/id", func(w http.ResponseWriter, r *http.Request) {
		idHandler(w, r, gos)
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		uploadHandler(w, r, gos)
	})
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		downloadHandler(w, r, gos)
	})
	mux.Handle("/", http.FileServer(http.Dir(DIR)))

	log.Printf("Serving %s on HTTP port: %s\n", DIR, UIPort)
	log.Fatal(http.ListenAndServe(":"+UIPort, mux))
}