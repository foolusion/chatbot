package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/net/websocket"
)

type self struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Created        int    `json:"created"`
	ManualPresence string `json:"manual_presence"`
}

type team struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	EmailDomain string `json:"email_domain"`
	Domain      string `json:"domain"`
}

var config = struct {
	token            string
	self             self
	team             team
	lastMsgTimestamp time.Time
}{
	lastMsgTimestamp: time.Now(),
}

func main() {
	log.SetOutput(os.Stdout)

	if os.Getenv("SLACK_TOKEN") == "" {
		log.Fatal("must supply SLACK_TOKEN environment variable")
	}
	config.token = os.Getenv("SLACK_TOKEN")

	url := callRTMStart()
	ws := createWSConn(url)

	errorChan := make(chan error)
	listenCtx, listenCancel := context.WithCancel(context.Background())
	go func() {
		defer listenCancel()
		errorChan <- listen(listenCtx, ws)
	}()
	pingerCtx, pingerCancel := context.WithCancel(context.Background())
	go func() {
		defer pingerCancel()
		errorChan <- pinger(pingerCtx, ws)
	}()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errorChan:
		if err != nil {
			log.Fatal(err)
		}
		ws.Close()
		listenCancel()
		pingerCancel()
	case s := <-signalChan:
		log.Println(fmt.Sprintf("Captured %v. Exitting...", s))
		ws.Close()
		listenCancel()
		pingerCancel()
		os.Exit(0)
	}
}

func callRTMStart() string {
	resp, err := http.Get(fmt.Sprintf("https://slack.com/api/rtm.start?token=%v", config.token))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	var rtmStart struct {
		Ok   bool   `json:"ok"`
		URL  string `json:"url"`
		Self self   `json:"self"`
		Team team   `json:"team"`
	}
	if err := dec.Decode(&rtmStart); err != nil {
		log.Fatal(err)
	}

	if !rtmStart.Ok {
		log.Fatalf("rtm.start did not return ok: %v", rtmStart)
	}
	config.self, config.team = rtmStart.Self, rtmStart.Team
	return rtmStart.URL
}

func createWSConn(url string) *websocket.Conn {
	ws, err := websocket.Dial(url, "", "https://slack.com/")
	if err != nil {
		log.Fatalf("dialing slack websocket: %v")
	}
	return ws
}

func pinger(ctx context.Context, ws *websocket.Conn) error {
	tick := time.NewTicker(time.Second)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			if time.Since(config.lastMsgTimestamp) < 5*time.Second {
				continue
			}
			msg := struct {
				ID   int    `json:"id"`
				Type string `json:"type"`
				Time int64  `json:"time"`
			}{
				ID:   1,
				Type: "ping",
				Time: time.Now().Unix(),
			}
			websocket.JSON.Send(ws, msg)
		}
	}
	return nil
}

func listen(ctx context.Context, ws *websocket.Conn) error {
	ch := make(chan string)
	go func() {
		for {
			var msg string
			websocket.Message.Receive(ws, &msg)
			ch <- msg
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-ch:
			if err := handleMsg(msg); err != nil {
				return err
			}
		}
	}
	return nil
}

func handleMsg(msg string) error {
	var msgType struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(msg), &msgType); err != nil {
		return err
	}

	config.lastMsgTimestamp = time.Now()

	switch msgType.Type {
	case "hello":
		log.Println("It Works!")
	case "error":
		return handleError(msg)
	default:
		log.Println(msg)
	}
	return nil
}

func handleError(msg string) error {
	type slackError struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
	}
	var e struct {
		Error slackError `json:"error"`
	}
	if err := json.Unmarshal([]byte(msg), &e); err != nil {
		return fmt.Errorf("unmarshalling json: %v", err)
	}
	return fmt.Errorf("error listening: %v", e.Error.Message)
}
