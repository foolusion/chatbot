package main

import (
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

func (s *server) SendMessage(in *botrpc.ChatMessage, stream botrpc.Bot_SendMessageServer) error {
	return handleChat(in, stream)
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

var errorChan = make(chan error)

func main() {
	log.SetOutput(os.Stdout)
	if addr := os.Getenv("CHATBOT_ADDR"); addr != "" {
		config.addr = addr
	}

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
		log.Println(fmt.Sprintf("Captured %v. Exitting...", s))
		// shutdown incoming chat listener
		// shutdown registration server
		os.Exit(0)
	}
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

func handleChat(in *botrpc.ChatMessage, demux botrpc.Bot_SendMessageServer) error {
	if chatFuncs == nil {
		return nil
	}
	for _, cf := range chatFuncs {
		if ok := cf.triggerExpr.MatchString(in.Body); !ok {
			continue
		}

		conn, err := grpc.Dial(cf.Addr, grpc.WithInsecure())
		if err != nil {
			log.Printf("error connecting with client: %v", err)
		}
		defer conn.Close()
		c := botrpc.NewBotFuncsClient(conn)

		// TODO fix this
		in.FuncName = cf.FuncName
		stream, err := c.SendMessage(context.Background(), in)
		for {
			in, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("error streaming from BotFuncs: %v", err)
				break
			}
			if err := demux.Send(in); err == io.EOF {
				break
			} else if err != nil {
				log.Printf("error streaming to integration: %v", err)
				break
			}
		}
	}
	return nil
}
