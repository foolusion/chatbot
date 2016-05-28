package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/foolusion/chatbot/botrpc"
	"google.golang.org/grpc"

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
	ws               *websocket.Conn
	self             self
	team             team
	lastMsgTimestamp time.Time
	address          string
	client           botrpc.BotClient
}{
	lastMsgTimestamp: time.Now(),
}

func main() {
	log.SetOutput(os.Stdout)

	if os.Getenv("SLACK_TOKEN") == "" {
		log.Fatal("must set SLACK_TOKEN environment variable")
	}
	config.token = os.Getenv("SLACK_TOKEN")

	if os.Getenv("CHATBOT_ADDRESS") == "" {
		log.Fatal("must set CHATBOT_ADDRESS environment variable")
	}
	config.address = os.Getenv("CHATBOT_ADDRESS")

	if err := connectToChatbot(); err != nil {
		log.Fatal(err)
	}

	url := callRTMStart()
	config.ws = createWSConn(url)

	errorChan := make(chan error)
	listenCtx, listenCancel := context.WithCancel(context.Background())
	go func() {
		defer listenCancel()
		errorChan <- listen(listenCtx)
	}()
	pingerCtx, pingerCancel := context.WithCancel(context.Background())
	go func() {
		defer pingerCancel()
		errorChan <- pinger(pingerCtx)
	}()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case err := <-errorChan:
			if err != nil {
				log.Fatal(err)
			}
			continue
		case s := <-signalChan:
			log.Println(fmt.Sprintf("Captured %v. Exitting...", s))
			config.ws.Close()
			listenCancel()
			pingerCancel()
			os.Exit(0)
		}
	}
}

func connectToChatbot() error {
	conn, err := grpc.Dial(config.address, grpc.WithInsecure())
	if err != nil {
		return fmt.Errorf("error connecting with client: %v", err)
	}
	config.client = botrpc.NewBotClient(conn)
	return nil
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

func pinger(ctx context.Context) error {
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
			websocket.JSON.Send(config.ws, msg)
		}
	}
	return nil
}

func listen(ctx context.Context) error {
	ch := make(chan string)
	go func() {
		for {
			var msg string
			websocket.Message.Receive(config.ws, &msg)
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
	case "message":
		handleMessage(msg)
	default:
		log.Println(msg)
	}
	return nil
}

type slackMessage struct {
	Channel string `json:"channel,omitempty"`
	User    string `json:"user,omitempty"`
	Text    string `json:"text,omitempty"`
	Ts      string `json:"ts,omitempty"`
	Type    string `json:"type"`
}

func handleMessage(msg string) error {
	var sm slackMessage
	if err := json.Unmarshal([]byte(msg), &sm); err != nil {
		return err
	}
	m := &botrpc.ChatMessage{
		Body:    sm.Text,
		User:    sm.User,
		Channel: sm.Channel,
	}
	log.Printf("sending to chatbot: %v\n", m.Body)
	stream, err := config.client.SendMessage(context.Background(), m)
	if err != nil {
		return err
	}
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		log.Printf("received response from chatbot: %v", in.Body)
		websocket.JSON.Send(config.ws, slackMessage{Type: "message", Channel: in.Channel, Text: in.Body})
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
