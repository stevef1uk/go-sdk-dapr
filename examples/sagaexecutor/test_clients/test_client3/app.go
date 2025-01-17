package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"net/http"
	"os"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	dapr "github.com/dapr/go-sdk/client"
	service "github.com/dapr/go-sdk/examples/sagaexecutor/service"
)

const myTopic = "test-client2"

var client dapr.Client
var s service.Server

func callback(w http.ResponseWriter, r *http.Request) {
	var params service.Start_stop
	fmt.Printf("Callback invoked!\n")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	json.NewDecoder(r.Body).Decode(&params)

	// Here do what is necessary to recover this transaction)
	fmt.Printf("OOPS! transaction callback invoked %v\n\n", params)
	json.NewEncoder(w).Encode("ok")
}

func main() {
	var err error

	appPort := "6000"
	if value, ok := os.LookupEnv("APP_PORT"); ok {
		appPort = value
	}
	router := mux.NewRouter()

	log.Println("setting up handler")
	router.HandleFunc("/callback", callback).Methods("POST", "OPTIONS")
	go http.ListenAndServe(":"+appPort, router)

	log.Println("About to send a couple of messages")

	client, err = dapr.NewClient()
	if err != nil {
		panic(err)
	}
	defer client.Close()

	s = service.NewService(myTopic)
	defer s.CloseService()

	log.Println("Sleeping for a bit")
	time.Sleep(5 * time.Second)

	log.Println("Finished sleeping")

	// Now lets test some load

	log.Println("Sending a group of starts & stops")
	for i := 0; i < 100; i++ {
		token := uuid.NewString()
		err = s.SendStart(client, "mock-client2", "test3", token, "callback", `{"UNEXPECTED":ERROR}`, 20)
		if err != nil {
			log.Printf("First Publish error got %s", err)
		}
		err = s.SendStop(client, "mock-client2", "test3", token)
		if err != nil {
			log.Printf("First Stop publish  error got %s", err)
		}
	}
	log.Println("Finished sending starts & stops")
	log.Println("Sleeping for quite a bit to allow time to receive any callbacks")
	time.Sleep(60 * time.Second)

	client.Close()

}
