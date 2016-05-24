package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"regexp"
	"syscall"

	"golang.org/x/net/context"

	"google.golang.org/grpc"

	"github.com/foolusion/chatbot/botrpc"
)

type server struct{}

func (s *server) Add(ctx context.Context, in *botrpc.Func) (*botrpc.FuncStatus, error) {
	re, err := regexp.Compile(in.Trigger)
	if err != nil {
		return &botrpc.FuncStatus{
			Status: 0,
		}, err
	}
	cf := chatfunc{Func: *in, triggerExpr: re}
	chatFuncs = append(chatFuncs, cf)
	return &botrpc.FuncStatus{
		Status: 1,
	}, nil
}

func (s *server) Remove(ctx context.Context, in *botrpc.Func) (*botrpc.FuncStatus, error) {
	return nil, nil
}

type chatfunc struct {
	botrpc.Func
	triggerExpr *regexp.Regexp
}

var chatFuncs []chatfunc

var config = struct {
	addr string
}{
	addr: "0.0.0.0:8173",
}

func main() {
	log.SetOutput(os.Stdout)
	if addr := os.Getenv("CHATBOT_ADDR"); addr != "" {
		config.addr = addr
	}
	// connect to chat

	// listen for incoming chat
	errorChan := make(chan error)
	go func() {
		errorChan <- listen(os.Stdin)
	}()

	// start registration server
	go func() {
		errorChan <- startBotServer()
	}()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	select {
	case e := <-errorChan:
		log.Fatalf("error occurred: %v", e)
	case s := <-signalChan:
		fmt.Printf("Captured %v. Exitting...", s)
		// shutdown incoming chat listener
		// shutdown registration server
		os.Exit(0)
	}
}

func listen(r io.Reader) error {
	s := bufio.NewScanner(r)
	for s.Scan() {
		handleChat(s.Text())
	}
	if s.Err() == io.EOF {
		return fmt.Errorf("Captured %v on chat listener. Exitting...", s.Err())
	}
	if s.Err() != nil {
		return fmt.Errorf("scanning messages: %v", s.Err())
	}
	return nil
}
func startBotServer() error {
	lis, err := net.Listen("tcp", config.addr)
	if err != nil {
		log.Printf("failed to listen: %v\n", err)
	}
	s := grpc.NewServer()
	botrpc.RegisterBotServer(s, &server{})
	return s.Serve(lis)
}

func handleChat(msg string) {
	if chatFuncs == nil {
		return
	}
	for _, cf := range chatFuncs {
		if ok := cf.triggerExpr.MatchString(msg); !ok {
			continue
		}

		conn, err := grpc.Dial(cf.Addr, grpc.WithInsecure())
		if err != nil {
			log.Printf("error connecting with client: %v", err)
		}
		defer conn.Close()
		c := botrpc.NewBotFuncsClient(conn)

		stream, err := c.Run(context.Background(), &botrpc.ChatMessage{
			Body:     msg,
			Channel:  "main",
			User:     "andrew",
			FuncName: cf.FuncName,
		})
		for {
			in, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("error streaming from BotFuncs: %v", err)
				break
			}
			fmt.Fprintln(os.Stdout, in.Body)
		}
	}
}
