package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
)

// DIR base directory of static files
const DIR = "./static/"

// PrivateHTTPMessage structure with Text and Destination strings
type PrivateHTTPMessage struct {
	Text        string
	Destination string
}

// DownloadHTTPRequest structure with Name, Hash and Peer strings
type DownloadHTTPRequest struct {
	Name string
	Hash string
	Peer string
}

// SearchHTTPRequest structure with Keywords, Budget strings
type SearchHTTPRequest struct {
	Keywords string
	Budget   string
}

/* HTTP HANDLERS */

// /nodes handler
func nodeHandler(w http.ResponseWriter, r *http.Request, gos *Gossiper) {
	// If GET send the peers in json format
	if r.Method == "GET" {
		gos.mutexs.peersMutex.Lock()
		peers := NodesResponse{gos.peers}
		gos.mutexs.peersMutex.Unlock()

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
		gos.mutexs.peersMutex.Lock()
		if !strings.Contains(gos.peers, addr) {
			if gos.peers == "" {
				gos.peers = addr
			} else {
				auxPeers := strings.Split(gos.peers, ",")
				auxPeers = append(auxPeers, addr)
				gos.peers = strings.Join(auxPeers, ",")
			}
		}
		gos.mutexs.peersMutex.Unlock()

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("200 OK"))

	}
}

// messages handler
func messageHandler(w http.ResponseWriter, r *http.Request, gos *Gossiper) {
	// If GET send the gossiper messages in json format
	if r.Method == "GET" {
		gos.mutexs.rumorsMutex.Lock()
		messages := MessagesResponse{gos.rumors}
		gos.mutexs.rumorsMutex.Unlock()

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
			sendNewRumorMessage(gos, tmpMsg, nil)
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

		gos.mutexs.privateMutex.Lock()
		private := PrivateResponse{gos.private[user]}
		gos.mutexs.privateMutex.Unlock()

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

		tmpMsg := Message{
			Text:    "",
			File:    &data.Name,
			Request: &hash,
		}

		if data.Peer == "" {

			var searchMatch *SearchMatch

			for _, searchResult := range gos.SearchResult {
				if bytes.Compare(searchResult.MetafileHash, *tmpMsg.Request) == 0 {
					searchMatch = &searchResult
					break
				}
			}

			if searchMatch == nil {
				fmt.Println("File with hash " + hex.EncodeToString(*tmpMsg.Request) + " not found in a previous search")
				return
			}

			ch := make(chan DataReply)
			gos.mutexs.downloadsMutex.Lock()
			gos.downloads = append(gos.downloads, ch)
			gos.mutexs.downloadsMutex.Unlock()
			go handleFileDownloadFromSearch(gos, searchMatch, *tmpMsg.File, ch)

		} else {
			tmpMsg.Destination = &data.Peer

			ch := make(chan DataReply)
			gos.mutexs.downloadsMutex.Lock()
			gos.downloads = append(gos.downloads, ch)
			gos.mutexs.downloadsMutex.Unlock()
			go handleFileDownload(gos, tmpMsg, ch)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("200 OK"))
	}
}

func confirmedHandler(w http.ResponseWriter, r *http.Request, gos *Gossiper) {
	if r.Method == "GET" {
		res := ConfirmedTLCResponse{gos.confirmedTLCs}

		js, err := json.Marshal(res)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	}
}

func consensusHandler(w http.ResponseWriter, r *http.Request, gos *Gossiper) {
	if r.Method == "GET" {
		res := ConsensusResponse{gos.consensusOn}

		js, err := json.Marshal(res)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	}
}

// search handler
func searchHandler(w http.ResponseWriter, r *http.Request, gos *Gossiper) {
	// Iniciate a search
	if r.Method == "POST" {
		reqBody, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Fatal(err)
		}

		data := &SearchHTTPRequest{}

		_ = json.Unmarshal(reqBody, data)

		tmpMsg := Message{
			Text:     "",
			Keywords: &data.Keywords,
		}

		fmt.Println(data)

		if data.Budget != "0" {
			ui, _ := strconv.Atoi(data.Budget)
			ui64 := uint64(ui)
			tmpMsg.Budget = &ui64
		}

		tmpKeywords := strings.Split(*tmpMsg.Keywords, ",")
		sort.Strings(tmpKeywords)

		ch := make(chan SearchReply)
		gos.searchs = append(gos.searchs, ch)

		go newFileSearch(gos, tmpKeywords, tmpMsg.Budget, ch)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("200 OK"))
		// Return the current matches
	} else if r.Method == "GET" {

		searchMatches := SearchMatchResponse{gos.searchMatches}

		js, err := json.Marshal(searchMatches)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	}
}

// search handler
func fileSearchedHandler(w http.ResponseWriter, r *http.Request, gos *Gossiper) {
	// Return the search result
	if r.Method == "GET" {

		var searchMatchRes []SearchMatch
		var searchMatchHash []string

		for _, v := range gos.SearchResult {
			searchMatchRes = append(searchMatchRes, v)
			searchMatchHash = append(searchMatchHash, hex.EncodeToString(v.MetafileHash))
		}

		fileSearched := FileSearchedResponse{searchMatchRes, searchMatchHash}


		js, err := json.Marshal(fileSearched)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	}
}

// users handler
func usersHandler(w http.ResponseWriter, r *http.Request, gos *Gossiper) {
	// If GET send the nodes in json format
	if r.Method == "GET" {
		gos.mutexs.routingMutex.Lock()
		routingMap := gos.routingTable
		gos.mutexs.routingMutex.Unlock()
		keys := make([]string, 0, len(routingMap))
		for k := range routingMap {
			keys = append(keys, k)
		}
		peers := NodesResponse{strings.Join(keys, ",")}

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

// Round handler
func roundHandler(w http.ResponseWriter, r *http.Request, gos *Gossiper) {
	// Return the current round
	if r.Method == "GET" {

		messages := RoundResponse{
			Round:    int(atomic.LoadUint32(gos.myTime)),
			Advances: gos.roundAdvances,
		}

		js, err := json.Marshal(messages)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
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
	mux.HandleFunc("/round", func(w http.ResponseWriter, r *http.Request) {
		roundHandler(w, r, gos)
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		uploadHandler(w, r, gos)
	})
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		downloadHandler(w, r, gos)
	})
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		searchHandler(w, r, gos)
	})
	mux.HandleFunc("/fileSearched", func(w http.ResponseWriter, r *http.Request) {
		fileSearchedHandler(w, r, gos)
	})
	mux.HandleFunc("/confirmed", func(w http.ResponseWriter, r *http.Request) {
		confirmedHandler(w, r, gos)
	})
	mux.HandleFunc("/consensus", func(w http.ResponseWriter, r *http.Request) {
		consensusHandler(w, r, gos)
	})
	mux.Handle("/", http.FileServer(http.Dir(DIR)))

	log.Printf("Serving %s on HTTP port: %s\n", DIR, UIPort)
	log.Fatal(http.ListenAndServe(":"+UIPort, mux))
}
