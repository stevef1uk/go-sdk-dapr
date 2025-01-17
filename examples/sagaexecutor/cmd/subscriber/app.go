// Listen to a topic and store the messages in the Dapr StateStore
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	dapr "github.com/dapr/go-sdk/client"
	"github.com/dapr/go-sdk/examples/sagaexecutor/database"
	"github.com/dapr/go-sdk/examples/sagaexecutor/encodedecode"
	service "github.com/dapr/go-sdk/examples/sagaexecutor/service"
	"github.com/dapr/go-sdk/examples/sagaexecutor/utility"
	common "github.com/dapr/go-sdk/service/common"
	daprd "github.com/dapr/go-sdk/service/http"
)

const stateStoreComponentName = "sagalogs"

type dataElement struct {
	Data    string             `json:"data"`
	LogData utility.Start_stop `json:"logdata"`
}

var (
	sub = &common.Subscription{
		PubsubName: service.PubsubComponentName,
		Topic:      "Dummy-Not-Used",
		Route:      "/receivemessage",
	}
	sub_client  dapr.Client
	logger      = log.New(os.Stdout, "", 0)
	the_service service.Server
)

func closeAll() {
	sub_client.Close()
	the_service.CloseService()
}

func multiSignalHandler(signal os.Signal) {

	switch signal {
	case syscall.SIGHUP:
		logger.Println("Signal:", signal.String())
		closeAll()
		os.Exit(0)
	case syscall.SIGINT:
		closeAll()
		logger.Println("Signal:", signal.String())
		os.Exit(0)
	case syscall.SIGTERM:
		logger.Println("Signal:", signal.String())
		closeAll()
		os.Exit(0)
	case syscall.SIGQUIT:
		closeAll()
		logger.Println("Signal:", signal.String())
		os.Exit(0)
	default:
		logger.Println("Unhandled/unknown signal")
	}
}

func main() {
	var err error
	appPort := os.Getenv("APP_PORT")
	if appPort == "" {
		appPort = "7005"
	}

	the_service = service.NewService("") // Subscriber doesn't send messages to a topic just read them
	defer the_service.CloseService()

	sub_client, err = dapr.NewClient()
	if err != nil {
		panic(err)
	}
	defer sub_client.Close()

	// Create the new server on appPort and add a topic listener
	s := daprd.NewService(":" + appPort)
	err = s.AddTopicEventHandler(sub, eventHandler)
	if err != nil {
		log.Fatalf("error adding topic subscription: %v", err)
	}

	sigchnl := make(chan os.Signal, 1)
	signal.Notify(sigchnl, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM) //we can add more sycalls.SIGQUIT etc.
	exitchnl := make(chan int)

	go func() {
		for {
			s := <-sigchnl
			multiSignalHandler(s)
		}
	}()

	//log.Printf("Starting the server using port %s'n", appPort)
	// Start the server
	err = s.Start()
	if err != nil && err != http.ErrServerClosed {
		sub_client.Close()
		log.Fatalf("error listenning: %v", err)
	}
	sub_client.Close()

	exitcode := <-exitchnl
	os.Exit(exitcode)
}

func storeMessage(client dapr.Client, m *utility.Start_stop) error {
	var err error

	log.Printf("storeMessage m = %v\n", m)

	key := m.App_id + m.Service + m.Token

	// Only store Starts
	if m.Event == utility.Start {
		m.LogTime = time.Now().UTC()
		data, err := json.Marshal(m)
		if err != nil {
			log.Printf("storeMessage error marshalling %v, err = %s\n", m, err)
		}

		// Save state into the state store
		err = the_service.StoreStateEntry(key, []byte(data))
		if err != nil {
			log.Fatal(err)
		}
	} else { // Stop means we delete the corresponding Start entry
		// Delete state from the state store
		err = the_service.DeleteStateEntry(key) // Yes I really want to delete the Start record now!
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Deleted Log with key %s\n", key)
	}

	return err
}

func eventHandler(ctx context.Context, e *common.TopicEvent) (retry bool, err error) {
	//return false, err // Uncomment this to flush through queues if necessary for testing
	var m map[string]interface{} = e.Data.(map[string]interface{})

	fmt.Printf("eventHandler Ordering Key = %s\n", m["OrderingKey"].(string))

	tmp := &database.StateRecord{Key: "", Value: m["Data"].(string)}
	tmp.Value = encodedecode.DecodeData((tmp.Value))
	fmt.Printf("eventHandler decoded data = %s\n", tmp.Value)
	message := utility.ProcessRecord(*tmp, true)
	message.LogTime, _ = time.Parse(utility.ExpiryDateLayout, time.Now().String())

	log.Printf("eventHandler: Message:%v\n", message)

	err = storeMessage(sub_client, &message)
	if err != nil {
		log.Fatalf("Unable to store message %s", err)
	}

	return false, err
}
