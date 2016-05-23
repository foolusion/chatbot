package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/foolusion/chatbot/chatfunc"
)

func init() {
	http.HandleFunc("/register", register)
}

var chatFuncs []*chatfunc.Data

func main() {
	// connect to chat

	// listen for incoming chat
	go listen(os.Stdin)

	// start registration server
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func listen(r io.Reader) {
	s := bufio.NewScanner(r)
	for s.Scan() {
		handleChat(s.Text())
	}
	if s.Err() != nil {
		fmt.Fprintf(os.Stdout, "scanning messages: %v\n", s.Err())
	}
}

func register(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(r.Body)
	var cf chatfunc.Data
	if err := dec.Decode(&cf); err != nil {
		fmt.Fprintf(os.Stdout, "decoding register chatFunc: %v\n", err)
	}
	chatFuncs = append(chatFuncs, &cf)
}

func handleChat(msg string) {
	if chatFuncs == nil {
		return
	}
	for i, cf := range chatFuncs {
		ok, err := cf.Match(msg)
		if err != nil {
			fmt.Fprintf(os.Stdout, "error matching trigger: %v\n", err)
			chatFuncs[i] = chatFuncs[len(chatFuncs)-1]
			chatFuncs[len(chatFuncs)-1] = nil
			chatFuncs = chatFuncs[:len(chatFuncs)-1]
		}
		if !ok {
			continue
		}
		resp, err := http.Get(cf.Endpoint)
		if err != nil {
			fmt.Fprintf(os.Stdout, "response from chatFunc: %v\n", err)
		} else if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stdout, "non ok response from chatFunc: %v\n", resp.Status)
		}
		// write resp to chat conn
		io.Copy(os.Stdout, resp.Body)
	}
}
