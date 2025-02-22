package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"
)

// UPDIR upload directory
const UPDIR = "./_SharedFiles/"

// DOWNDIR download directory
const DOWNDIR = "./_Downloads/"

// CHUNKDIR Auxiliary folder inside SharedFiles used to store the different chunks of a file
const CHUNKDIR = UPDIR + "/_IndexedFilesChunks/"

// CHUNKSIZE chunk maximum size
const CHUNKSIZE = 8192

// SHASIZE size of the SHA digest
const SHASIZE = 32

// HOPLIMIT private messages limit
const HOPLIMIT = 10

// Index a new file
func handleFileIndexing(gos *Gossiper, filename string) {

	// Open the file in the UPDIR folder
	file, err := os.Open(UPDIR + filename)

	if err != nil {
		fmt.Println("File error: " + err.Error())
		return
	}

	defer file.Close()

	fi, err := file.Stat()

	if err != nil {
		fmt.Println("Stats error: " + err.Error())
		return
	}

	var hash []byte
	buffer := make([]byte, CHUNKSIZE)

	var chunkCount uint64
	chunkCount = 0
	var chunkMap []uint64

	// Read the file in step of CHUNKSIZE bytes
	for {
		bytesread, err := file.Read(buffer)

		if err != nil {
			if err != io.EOF {
				fmt.Println("Read error: " + err.Error())
			}

			break
		}

		// Compute the SHA256 and append it to hash[]
		sha := sha256.Sum256(buffer[:bytesread])
		hash = append(hash, sha[:]...)

		// Check that CHUNKDIR exists or create it
		if _, err := os.Stat(CHUNKDIR); os.IsNotExist(err) {
			os.Mkdir(CHUNKDIR, os.ModePerm)
		}

		// Write the chunk in chunkdir to speed up the download process
		f, err := os.Create(CHUNKDIR + hex.EncodeToString(sha[:]) + ".part")

		if err != nil {
			fmt.Println("File error: " + err.Error())
		}

		chunkCount++
		chunkMap = append(chunkMap, chunkCount)

		f.Write(buffer[:bytesread])
		f.Close()

	}

	// Add the fileIndex to the gossiper list of indexed files
	fileIndex := FileIndex{filename, fi.Size(), hash, sha256.Sum256(hash), chunkCount, chunkMap}

	if gos.hw3.hw3ex2 || gos.hw3.hw3ex3 || gos.hw3.hw3ex4 {
		listFile(gos, fileIndex.Name, fileIndex.Size, fileIndex.MetaHash[:])
	}

	gos.mutexs.filesIndexMutex.Lock()
	gos.filesIndex[fileIndex.MetaHash] = fileIndex
	gos.mutexs.filesIndexMutex.Unlock()
}

// Download thread
func handleFileDownload(gos *Gossiper, req Message, ch chan DataReply) {

	var meta *[]byte
	var chunkHash []byte
	var fileIndex FileIndex
	chunkCount := 1

	// Get the routing address
	gos.mutexs.routingMutex.Lock()
	addr := gos.routingTable[*req.Destination]
	gos.mutexs.routingMutex.Unlock()

	if addr != "" {
		// Send the metafile request
		rmr := &DataRequest{
			Origin:      gos.ID,
			Destination: *req.Destination,
			HopLimit:    HOPLIMIT - 1,
			HashValue:   *req.Request,
		}

		msg := &GossipPacket{DataRequest: rmr}

		sendMsgTo(addr, msg, *gos)
		printDownloadingMetafile(*req.File, *req.Destination)
	} else {
		fmt.Println("Unable to send message to unknown peer")
		return
	}

	for {
		// Start a new time for the metafile request
		timer := time.NewTimer(time.Second * 5)
		select {
		// Get a new reply from the channel
		case dataReply := <-ch:

			// In case we receive a reply for a metafile or a chunk that is empty (meaning the peer doens't have it) we delete the channel for the gossiper list and return from the thread
			if len(dataReply.Data) == 0 && (bytes.Compare(dataReply.HashValue, *req.Request) == 0 || bytes.Compare(dataReply.HashValue, chunkHash) == 0) {
				timer.Stop()
				i := -1
				gos.mutexs.downloadsMutex.Lock()
				for k, v := range gos.downloads {
					if v == ch {
						i = k
					}
				}

				if i != -1 {
					gos.downloads = append(gos.downloads[:i], gos.downloads[i+1:]...)
				}
				gos.mutexs.downloadsMutex.Unlock()
				fmt.Println(*req.Destination + " unable to send the requested chunk/metafile. Download aborted for file " + *req.File)
				return
			}

			// Compute the hash of the reply
			hashVal := sha256.Sum256(dataReply.Data)

			// Check if we already have the metafile
			if meta == nil {
				// Check that the reply is for this thread download
				if bytes.Compare(dataReply.HashValue, *req.Request) == 0 && bytes.Compare(hashVal[:], *req.Request) == 0 {

					// Stop the timer, get metafile and define first chunk request
					timer.Stop()
					meta = &dataReply.Data
					chunkHash = (*meta)[:32]

					fileIndex = FileIndex{*req.File, 0, dataReply.Data, hashVal, uint64(len(dataReply.Data) / 32), make([]uint64, 0)}
					gos.mutexs.filesIndexMutex.Lock()
					gos.filesIndex[fileIndex.MetaHash] = fileIndex
					gos.mutexs.filesIndexMutex.Unlock()

					// Check that DOWNDIR exists or create it
					if _, err := os.Stat(DOWNDIR); os.IsNotExist(err) {
						os.Mkdir(DOWNDIR, os.ModePerm)
					}

					// Create the file were file will be stored
					f, err := os.Create(DOWNDIR + *req.File)

					if err != nil {
						fmt.Println("File error: " + err.Error())
					}

					f.Close()

					// Send the chunk request and restart the timer
					rmr := &DataRequest{
						Origin:      gos.ID,
						Destination: *req.Destination,
						HopLimit:    HOPLIMIT - 1,
						HashValue:   chunkHash,
					}

					msg := &GossipPacket{DataRequest: rmr}

					sendMsgTo(addr, msg, *gos)
					timer.Reset(time.Second * 5)

					printDownloadingChunk(*req.File, *req.Destination, chunkCount)
					chunkCount++
				}
			} else {
				if bytes.Compare(dataReply.HashValue, chunkHash) == 0 && bytes.Compare(hashVal[:], chunkHash) == 0 {

					timer.Stop()
					fileIndex.Size += int64(len(dataReply.Data))
					fileIndex.ChunkMap = append(fileIndex.ChunkMap, uint64(chunkCount-1))
					gos.mutexs.filesIndexMutex.Lock()
					gos.filesIndex[fileIndex.MetaHash] = fileIndex
					gos.mutexs.filesIndexMutex.Unlock()

					// Write the file with the chunk
					file, err := os.OpenFile(DOWNDIR+*req.File, os.O_APPEND|os.O_WRONLY, os.ModeAppend)

					if err != nil {
						fmt.Println("File error: " + err.Error())
					}

					file.Write(dataReply.Data)
					file.Close()

					// Check that CHUNKDIR exists or create it
					if _, err := os.Stat(CHUNKDIR); os.IsNotExist(err) {
						os.Mkdir(CHUNKDIR, os.ModePerm)
					}

					// Write the chunk in chunkdir to speed up the download process
					f, err := os.Create(CHUNKDIR + hex.EncodeToString(dataReply.HashValue) + ".part")

					if err != nil {
						fmt.Println("File error: " + err.Error())
					}

					f.Write(dataReply.Data)
					f.Close()

					// Check if the metafile has more chunk hashes or the download has finished
					if len(*meta) == 32 {
						printReconstructed(*req.File)
						i := -1

						// Remove the channel from the gossiper and return from the thread
						gos.mutexs.downloadsMutex.Lock()
						for k, v := range gos.downloads {
							if v == ch {
								i = k
							}
						}

						if i != -1 {
							gos.downloads = append(gos.downloads[:i], gos.downloads[i+1:]...)
						}
						gos.mutexs.downloadsMutex.Unlock()
						return
					}

					// Get the next chunk to be downloaded and send its request
					tmp := (*meta)[32:]
					meta = &tmp
					chunkHash = (*meta)[:32]

					rmr := &DataRequest{
						Origin:      gos.ID,
						Destination: *req.Destination,
						HopLimit:    HOPLIMIT - 1,
						HashValue:   chunkHash,
					}

					msg := &GossipPacket{DataRequest: rmr}

					sendMsgTo(addr, msg, *gos)
					timer.Reset(time.Second * 5)

					printDownloadingChunk(*req.File, *req.Destination, chunkCount)
					chunkCount++
				}
			}
		// If timer ticks resend the last request sent wheter it was for a chunk or for the metafile (and restart the timer)
		case <-timer.C:
			if meta == nil {
				rmr := &DataRequest{
					Origin:      gos.ID,
					Destination: *req.Destination,
					HopLimit:    HOPLIMIT - 1,
					HashValue:   *req.Request,
				}

				msg := &GossipPacket{DataRequest: rmr}

				sendMsgTo(addr, msg, *gos)
				timer.Reset(time.Second * 5)
			} else {
				rmr := &DataRequest{
					Origin:      gos.ID,
					Destination: *req.Destination,
					HopLimit:    HOPLIMIT - 1,
					HashValue:   chunkHash,
				}

				msg := &GossipPacket{DataRequest: rmr}

				sendMsgTo(addr, msg, *gos)
				timer.Reset(time.Second * 5)
			}
		}
	}
}

// Download thread
func handleFileDownloadFromSearch(gos *Gossiper, searchMatch *SearchMatch, name string, ch chan DataReply) {

	var meta *[]byte
	var chunkHash []byte
	var fileIndex FileIndex
	chunkCount := 1

	currentNode := ""
	for i, val := range searchMatch.Chunks {
		if val == uint64(chunkCount) {
			currentNode = searchMatch.Nodes[i]
		}
	}

	// Get the routing address from the first chunk
	gos.mutexs.routingMutex.Lock()
	addr := gos.routingTable[currentNode]
	gos.mutexs.routingMutex.Unlock()

	if addr != "" {

		// Send the metafile request
		rmr := &DataRequest{
			Origin:      gos.ID,
			Destination: currentNode,
			HopLimit:    HOPLIMIT - 1,
			HashValue:   searchMatch.MetafileHash,
		}

		msg := &GossipPacket{DataRequest: rmr}

		sendMsgTo(addr, msg, *gos)
		printDownloadingMetafile(name, currentNode)
	} else {
		fmt.Println("Unable to send message to unknown peer")
		return
	}

	for {
		// Start a new time for the metafile request
		timer := time.NewTimer(time.Second * 5)
		select {
		// Get a new reply from the channel
		case dataReply := <-ch:

			// In case we receive a reply for a metafile or a chunk that is empty (meaning the peer doens't have it) we delete the channel for the gossiper list and return from the thread
			if len(dataReply.Data) == 0 && (bytes.Compare(dataReply.HashValue, searchMatch.MetafileHash) == 0 || bytes.Compare(dataReply.HashValue, chunkHash) == 0) {
				timer.Stop()
				i := -1
				gos.mutexs.downloadsMutex.Lock()
				for k, v := range gos.downloads {
					if v == ch {
						i = k
					}
				}

				if i != -1 {
					gos.downloads = append(gos.downloads[:i], gos.downloads[i+1:]...)
				}
				gos.mutexs.downloadsMutex.Unlock()
				fmt.Println(currentNode + " unable to send the requested chunk/metafile. Download aborted for file " + name)
				return
			}

			// Compute the hash of the reply
			hashVal := sha256.Sum256(dataReply.Data)

			// Check if we already have the metafile
			if meta == nil {
				// Check that the reply is for this thread download
				if bytes.Compare(dataReply.HashValue, searchMatch.MetafileHash) == 0 && bytes.Compare(hashVal[:], searchMatch.MetafileHash) == 0 {

					// Stop the timer, get metafile and define first chunk request
					timer.Stop()
					meta = &dataReply.Data
					chunkHash = (*meta)[:32]

					fileIndex = FileIndex{name, 0, dataReply.Data, hashVal, uint64(len(dataReply.Data) / 32), make([]uint64, 0)}
					gos.mutexs.filesIndexMutex.Lock()
					gos.filesIndex[fileIndex.MetaHash] = fileIndex
					gos.mutexs.filesIndexMutex.Unlock()

					// Check that DOWNDIR exists or create it
					if _, err := os.Stat(DOWNDIR); os.IsNotExist(err) {
						os.Mkdir(DOWNDIR, os.ModePerm)
					}

					// Create the file were file will be stored
					f, err := os.Create(DOWNDIR + name)

					if err != nil {
						fmt.Println("File error: " + err.Error())
					}

					f.Close()

					// Send the chunk request and restart the timer
					rmr := &DataRequest{
						Origin:      gos.ID,
						Destination: currentNode,
						HopLimit:    HOPLIMIT - 1,
						HashValue:   chunkHash,
					}

					msg := &GossipPacket{DataRequest: rmr}

					sendMsgTo(addr, msg, *gos)
					timer.Reset(time.Second * 5)

					printDownloadingChunk(name, currentNode, chunkCount)
					chunkCount++
				}
			} else {
				if bytes.Compare(dataReply.HashValue, chunkHash) == 0 && bytes.Compare(hashVal[:], chunkHash) == 0 {

					timer.Stop()
					fileIndex.Size += int64(len(dataReply.Data))
					fileIndex.ChunkMap = append(fileIndex.ChunkMap, uint64(chunkCount-1))
					gos.mutexs.filesIndexMutex.Lock()
					gos.filesIndex[fileIndex.MetaHash] = fileIndex
					gos.mutexs.filesIndexMutex.Unlock()

					// Write the file with the chunk
					file, err := os.OpenFile(DOWNDIR+name, os.O_APPEND|os.O_WRONLY, os.ModeAppend)

					if err != nil {
						fmt.Println("File error: " + err.Error())
					}

					file.Write(dataReply.Data)
					file.Close()

					// Check that CHUNKDIR exists or create it
					if _, err := os.Stat(CHUNKDIR); os.IsNotExist(err) {
						os.Mkdir(CHUNKDIR, os.ModePerm)
					}

					// Write the chunk in chunkdir to speed up the download process
					f, err := os.Create(CHUNKDIR + hex.EncodeToString(dataReply.HashValue) + ".part")

					if err != nil {
						fmt.Println("File error: " + err.Error())
					}

					f.Write(dataReply.Data)
					f.Close()

					// Check if the metafile has more chunk hashes or the download has finished
					if len(*meta) == 32 {
						printReconstructed(name)
						i := -1

						// Remove the channel from the gossiper and return from the thread
						gos.mutexs.downloadsMutex.Lock()
						for k, v := range gos.downloads {
							if v == ch {
								i = k
							}
						}

						if i != -1 {
							gos.downloads = append(gos.downloads[:i], gos.downloads[i+1:]...)
						}
						gos.mutexs.downloadsMutex.Unlock()
						return
					}

					// Get the next chunk to be downloaded and send its request
					tmp := (*meta)[32:]
					meta = &tmp
					chunkHash = (*meta)[:32]

					for i, val := range searchMatch.Chunks {
						if val == uint64(chunkCount) {
							currentNode = searchMatch.Nodes[i]
						}
					}

					rmr := &DataRequest{
						Origin:      gos.ID,
						Destination: currentNode,
						HopLimit:    HOPLIMIT - 1,
						HashValue:   chunkHash,
					}

					msg := &GossipPacket{DataRequest: rmr}

					sendMsgTo(addr, msg, *gos)
					timer.Reset(time.Second * 5)

					printDownloadingChunk(name, currentNode, chunkCount)
					chunkCount++
				}
			}
		// If timer ticks resend the last request sent wheter it was for a chunk or for the metafile (and restart the timer)
		case <-timer.C:
			if meta == nil {
				rmr := &DataRequest{
					Origin:      gos.ID,
					Destination: currentNode,
					HopLimit:    HOPLIMIT - 1,
					HashValue:   searchMatch.MetafileHash,
				}

				msg := &GossipPacket{DataRequest: rmr}

				sendMsgTo(addr, msg, *gos)
				timer.Reset(time.Second * 5)
			} else {
				rmr := &DataRequest{
					Origin:      gos.ID,
					Destination: currentNode,
					HopLimit:    HOPLIMIT - 1,
					HashValue:   chunkHash,
				}

				msg := &GossipPacket{DataRequest: rmr}

				sendMsgTo(addr, msg, *gos)
				timer.Reset(time.Second * 5)
			}
		}
	}
}
