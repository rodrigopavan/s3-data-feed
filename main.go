package main

import (
	"log"
	"net/http"
)

func main() {
	broker := NewServer()
	startQueuePooling(broker)
	log.Fatal("HTTP server error: ", http.ListenAndServe("localhost:3000", broker))
}
