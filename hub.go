package main

import (
	"bytes"
	"encoding/json"
	"log"
)

type Msg = struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type HubMessage struct {
	Action  string   `json:"action"`
	Topics  []string `json:"topics"`
	Message Msg      `json:"message"`
}

func SendMe(text string) error {
	API := "https://hub.drink.cafe/http"
	msg := HubMessage{
		Action: "PUB",
		Topics: []string{"weaming"},
		Message: Msg{
			Type: "PLAIN", Data: text,
		},
	}
	bin, _ := json.Marshal(msg)
	res, e := client.Post(API, "application/json", bytes.NewBuffer(bin))
	if e != nil {
		log.Println(e)
		return e
	}
	if res != nil && res.Body != nil {
		defer res.Body.Close()
	}
	return nil
}
