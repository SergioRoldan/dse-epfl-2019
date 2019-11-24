package main

import (
	"encoding/hex"
	"reflect"
	"strings"
	"time"
)

// MATCHTH match threshold
const MATCHTH = 2

// MAXBUDGET maximum budget
const MAXBUDGET = 32

func handleDuplicateSearch(gos *Gossiper, origin string, kwords []string) {
	gos.recentSearchs[origin] = append(gos.recentSearchs[origin], kwords)

	ticker := time.NewTicker(500 * time.Millisecond)

	for {
		select {
		case <-ticker.C:
			for i, keywords := range gos.recentSearchs[origin] {
				if reflect.DeepEqual(keywords, kwords) {
					gos.recentSearchs[origin] = append(gos.recentSearchs[origin][:i], gos.recentSearchs[origin][i+1:]...)
					break
				}
			}

			return
		}
	}
}

func newFileSearch(gos *Gossiper, keywords []string, budget *uint64, ch chan SearchReply) {

	match := make(map[string]SearchMatch)

	if budget == nil {
		budg := 2
		ticker := time.NewTicker(time.Second)

		for {
			select {
			case <-ticker.C:

				if (budg * 2) == MAXBUDGET {
					ticker.Stop()
				}

				budg *= 2
				divideBudget(gos, uint64(budg), gos.ID, keywords)

			case searchReply := <-ch:
				for _, searchResult := range searchReply.Results {
					// As multiple searches at the same time are possible we check if a reply is for the current thread checking if the name contains at least one of the keywords
					fnd := false
					for _, kw := range keywords {
						if strings.Contains(searchResult.FileName, kw) {
							fnd = true
							break
						}
					}

					// In case the name does not contain at least one the keywords, the reply is for another node and we can skip this iteration
					if !fnd {
						continue
					}

					printMatchFound(gos, searchResult.FileName, searchReply.Origin, hex.EncodeToString(searchResult.MetafileHash), searchResult.ChunkMap)

					if _, exist := match[searchResult.FileName]; !exist {
						match[searchResult.FileName] = SearchMatch{
							FileName:     searchResult.FileName,
							MetafileHash: searchResult.MetafileHash,
							ChunkCount:   searchResult.ChunkCount,
						}
					}

					for _, chunk := range searchResult.ChunkMap {
						found := false
						for _, chnk := range match[searchResult.FileName].Chunks {
							if chnk == chunk {
								found = true
								break
							}
						}

						if !found {
							tmpSearchMatch := match[searchResult.FileName]
							tmpSearchMatch.Nodes = append(tmpSearchMatch.Nodes, searchReply.Origin)
							tmpSearchMatch.Chunks = append(tmpSearchMatch.Chunks, chunk)
							match[searchResult.FileName] = tmpSearchMatch
						}

					}

					matches := 0

					for _, v := range match {
						if int(v.ChunkCount) == len(v.Chunks) {
							matches++
							gos.SearchResult[searchResult.FileName] = match[searchResult.FileName]
						}
					}

					if matches >= MATCHTH {
						//Stop the ticker

						printSearchFinished()

						ticker.Stop()
						i := -1

						// Remove the channel from the gossiper and return from the thread
						for k, v := range gos.searchs {
							if v == ch {
								i = k
							}
						}

						if i != -1 {
							gos.searchs = append(gos.searchs[:i], gos.searchs[i+1:]...)
						}

						return
					}
				}
			}

		}
	} else {

		divideBudget(gos, uint64(*budget), gos.ID, keywords)

		ticker := time.NewTicker(2 * time.Second)

		for {
			select {
			case <-ticker.C:

				printSearchFinished()

				ticker.Stop()
				i := -1

				// Remove the channel from the gossiper and return from the thread
				for k, v := range gos.searchs {
					if v == ch {
						i = k
					}
				}

				if i != -1 {
					gos.searchs = append(gos.searchs[:i], gos.searchs[i+1:]...)
				}

				return
			case searchReply := <-ch:
				for _, searchResult := range searchReply.Results {

					// As multiple searches at the same time are possible we check if a reply is for the current thread checking if the name contains at least one of the keywords
					fnd := false
					for _, kw := range keywords {
						if strings.Contains(searchResult.FileName, kw) {
							fnd = true
							break
						}
					}

					// In case the name does not contain at least one the keywords, the reply is for another node and we can skip this iteration
					if !fnd {
						continue
					}

					printMatchFound(gos, searchResult.FileName, searchReply.Origin, hex.EncodeToString(searchResult.MetafileHash), searchResult.ChunkMap)

					if _, exist := match[searchResult.FileName]; !exist {
						match[searchResult.FileName] = SearchMatch{
							FileName:     searchResult.FileName,
							MetafileHash: searchResult.MetafileHash,
							ChunkCount:   searchResult.ChunkCount,
						}
					}

					for _, chunk := range searchResult.ChunkMap {
						found := false
						for _, chnk := range match[searchResult.FileName].Chunks {
							if chnk == chunk {
								found = true
								break
							}
						}

						if !found {
							tmpSearchMatch := match[searchResult.FileName]
							tmpSearchMatch.Nodes = append(tmpSearchMatch.Nodes, searchReply.Origin)
							tmpSearchMatch.Chunks = append(tmpSearchMatch.Chunks, chunk)
							match[searchResult.FileName] = tmpSearchMatch
						}

					}

					for _, v := range match {
						if int(v.ChunkCount) == len(v.Chunks) {
							gos.SearchResult[searchResult.FileName] = match[searchResult.FileName]
						}
					}

				}
			}

		}

	}
}

func divideBudget(gos *Gossiper, budget uint64, origin string, keywords []string) {
	if budget > 0 && len(strings.Split(gos.peers, ",")) > 0 {
		divBudget := int(int(budget) / len(strings.Split(gos.peers, ",")))
		modBudget := int(budget) % len(strings.Split(gos.peers, ","))

		for _, pr := range strings.Split(gos.peers, ",") {
			if modBudget > 0 {
				forwardSearchRequest(gos, origin, pr, keywords, uint64(divBudget+1))
				modBudget--
			} else if divBudget != 0 {
				forwardSearchRequest(gos, origin, pr, keywords, uint64(divBudget))
			} else {
				break
			}
		}
	}
}
