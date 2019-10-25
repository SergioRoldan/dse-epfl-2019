package main

import (
	"os"
	"fmt"
	"crypto/sha256"
	"io"
)

const FILESDIR = "./_SharedFiles/"
const CHUNKSIZE = 8000
const SHASIZE = 32

func handleFileIndexing(gos *Gossiper, filename string) {
	file, err := os.Open(FILESDIR + filename)

	if err != nil {
		fmt.Println("File error: " + err.Error())
	}

	defer file.Close()

	fi, err := file.Stat()
	
	if err != nil {
		fmt.Println("Stats error: " + err.Error())
	}

	var hash []byte
	chunks := make([][]byte, int(fi.Size() / CHUNKSIZE) + 1)
	buffer := make([]byte, CHUNKSIZE)

	i := 0
	for {
		bytesread, err := file.Read(buffer)

		if err != nil {
			if err != io.EOF {
				fmt.Println(err)
			}

			break
		}
		sha := sha256.Sum256(buffer[:bytesread])
		hash = append(hash, sha[:]...)
		chunks[i] = buffer[:bytesread]
		i++
	}

	fileIndex := FileIndex { filename, fi.Size(), hash, sha256.Sum256(hash)}

	gos.filesIndex[fileIndex.MetaHash] = fileIndex

	fmt.Println(gos.filesIndex)
}

func handleFileDownload(gos *Gossiper, msg *Message) {

}