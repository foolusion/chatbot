package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/foolusion/chatbot/chatfunc"
)

func init() {
	http.HandleFunc("/", rootHandler)
}

func main() {
	if err := register(); err != nil {
		fmt.Fprintf(os.Stdout, "error registering hellobot: %v", err)
	}

	log.Fatal(http.ListenAndServe(":8081", nil))
}

func register() error {
	cfd := chatfunc.Data{
		Endpoint: "http://localhost:8081",
		Trigger:  "hello",
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(&cfd); err != nil {
		return fmt.Errorf("encoding chatfunc: %v", err)
	}
	r, err := http.NewRequest("GET", "http://localhost:8080/register", &buf)
	if err != nil {
		return fmt.Errorf("creating request: %v", err)
	}
	c := &http.Client{}
	resp, err := c.Do(r)
	if err != nil {
		return fmt.Errorf("doing request: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
	return nil
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "hey there")
}
