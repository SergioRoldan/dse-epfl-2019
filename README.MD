Folder structure:
    ./_Dowloads: Contains the downloaded files
    ./_SharedFiles: Contains the local files to be indexed
        ./_IndexedFilesChunks: Auxiliary folder to store all the chunks of indexed files for efficiency reasons during download. This file could be emptied each time the gossiper restarts because all the files should be indexed again. It's a tradeoff between space (files are stored twice) and speed to serve files (we only need to read each chunk not the whole file every time)
    ./client: Contains the client src
    ./static: Contains the GUI (html, js, css and favicon)

Files in the root folder:
    main.go: Main file with the call to the gossiperHandler and clientHandler threads
    fileHandler.go: File with functions to handle the indexing and download of files
    gossiper.go: File with functions to handle the client and gossiper connections
    http.go: File with functions to handle http connections to the http server
    structs.go: File containing the shared structs of the gossiper
    routing.go: File with functions to handle routing messages
    sendNewMsg.go: File with functions to handle the transmissions of different messages
    fileSearch.go: File with the functions to handle a search request
    blockchain.go: File with the functions that correspond to the hw3 from ex 2 to 4