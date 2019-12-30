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

// Handles a duplicate search within 0.5 seconds
func handleDuplicateSearch(gos *Gossiper, origin string, kwords []string) {
	gos.mutexs.recentSearchsMutex.Lock()
	gos.recentSearchs[origin] = append(gos.recentSearchs[origin], kwords)
	gos.mutexs.recentSearchsMutex.Unlock()

	ticker := time.NewTicker(500 * time.Millisecond)

	for {
		select {
		case <-ticker.C:
			gos.mutexs.recentSearchsMutex.Lock()
			defer gos.mutexs.recentSearchsMutex.Unlock()

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

// Generates a new file search with the keywords specified
func newFileSearch(gos *Gossiper, keywords []string, budget *uint64, ch chan SearchReply) {

	match := make(map[string]SearchMatch)

	if budget == nil {
		budg := 2
		ticker := time.NewTicker(time.Second)
		matches := make(map[string]bool)

		for {
			select {
			case <-ticker.C:

				//In case of timeout we double the budget til it reaches 32
				if (budg * 2) == MAXBUDGET {
					ticker.Stop()
				}

				budg *= 2
				divideBudget(gos, uint64(budg-1), gos.ID, "", keywords)

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

					// In case the name does not contain at least one keyword, the reply is for another node and we can skip this iteration
					if !fnd {
						continue
					}

					// We generate the and fill the match acording to the chunks in the result
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

					// If the match is a full match we indicate it
					if int(match[searchResult.FileName].ChunkCount) == len(match[searchResult.FileName].Chunks) {
						matches[searchResult.FileName] = true
						gos.mutexs.searchResultMutex.Lock()
						gos.SearchResult[searchResult.FileName] = match[searchResult.FileName]
						gos.mutexs.searchResultMutex.Unlock()
					}
				}

				// In case we have as many full matches as threshold we end the search
				count := 0
				for _, v := range matches {
					if v {
						count++
					}
				}

				if count >= MATCHTH {
					printSearchFinished()
					deleteSearchChannel(gos, ticker, ch)
					return
				}

			}

		}
	} else {

		// Same case with a predefined budged
		divideBudget(gos, uint64(*budget-1), gos.ID, "", keywords)

		ticker := time.NewTicker(2 * time.Second)

		matches := make(map[string]bool)

		for {
			select {
			case <-ticker.C:

				deleteSearchChannel(gos, ticker, ch)

				return
			case searchReply := <-ch:
				for _, searchResult := range searchReply.Results {

					fnd := false
					for _, kw := range keywords {
						if strings.Contains(searchResult.FileName, kw) {
							fnd = true
							break
						}
					}

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

					if int(match[searchResult.FileName].ChunkCount) == len(match[searchResult.FileName].Chunks) {
						matches[searchResult.FileName] = true
						gos.mutexs.searchResultMutex.Lock()
						gos.SearchResult[searchResult.FileName] = match[searchResult.FileName]
						gos.mutexs.searchResultMutex.Unlock()
					}

				}

				count := 0
				for _, v := range matches {
					if v {
						count++
					}
				}

				if count >= MATCHTH {

					printSearchFinished()

					deleteSearchChannel(gos, ticker, ch)

					return
				}
			}

		}

	}
}

// Divide the budget between all the nodes (except the originator) equitatively
func divideBudget(gos *Gossiper, budget uint64, origin, address string, keywords []string) {

	peers := strings.Split(gos.peers, ",")

	if address != "" {
		j := -1
		for i, peer := range peers {
			if peer == address {
				j = i
				break
			}
		}

		if j >= 0 {
			peers = append(peers[:j], peers[j+1:]...)
		}
	}

	if budget > 0 && len(peers) > 0 {
		divBudget := int(int(budget) / len(peers))
		modBudget := int(budget) % len(peers)

		for _, pr := range peers {
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

// Delete the search channel from the list to garbage collect it
func deleteSearchChannel(gos *Gossiper, ticker *time.Ticker, ch chan SearchReply) {
	ticker.Stop()
	i := -1

	gos.mutexs.searchsMutex.Lock()

	for k, v := range gos.searchs {
		if v == ch {
			i = k
		}
	}

	if i != -1 {
		gos.searchs = append(gos.searchs[:i], gos.searchs[i+1:]...)
	}

	gos.mutexs.searchsMutex.Unlock()

	return
}
