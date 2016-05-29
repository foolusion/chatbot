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

// server is used to implement the BotServer interface.
type server struct{}

// Add adds a function to the server. This should be called for each function
// that a bot can respond.
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

// Remove deletes the func from server so it will no longer trigger.
// This doesn't seem neccessary the more i think about it. I can't think of a
// case where you would remove a function from a server.
func (s *server) Remove(ctx context.Context, in *botrpc.Func) (*botrpc.FuncStatus, error) {
	// TODO: implement this.
	return nil, nil
}

// SendMessage recieves messages from the integrations and handles them.
func (s *server) SendMessage(in *botrpc.ChatMessage, stream botrpc.Bot_SendMessageServer) error {
	return handleChat(in, stream)
}

// chatfunc is a botrpc.Func  with the compiled regular expression.
type chatfunc struct {
	botrpc.Func
	triggerExpr *regexp.Regexp
}

// chatFuncs contains all the registered botrpc.Func with compiled regular
// expressions.
var chatFuncs []chatfunc

// config is a convenient group for global variables.
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

// startBotServer listens on the address specified in config.addr and handles
// rpcs. Have to check if we can store the server for draining and closing the
// connection.
func startBotServer() error {
	lis, err := net.Listen("tcp", config.addr)
	if err != nil {
		log.Printf("failed to listen: %v\n", err)
	}
	s := grpc.NewServer()
	botrpc.RegisterBotServer(s, &server{})
	return s.Serve(lis)
}

// handleChat checks if any bots are triggered and sends all the responses back
// on outStream.
func handleChat(in *botrpc.ChatMessage, outStream botrpc.Bot_SendMessageServer) error {
	if chatFuncs == nil {
		return nil
	}
	// TODO: handle help

	// for each func check if they are triggered
	for _, cf := range chatFuncs {
		if ok := cf.triggerExpr.MatchString(in.Body); !ok {
			continue
		}

		// create a connection to the bot
		conn, err := grpc.Dial(cf.Addr, grpc.WithInsecure())
		if err != nil {
			log.Printf("error connecting with client: %v", err)
		}
		defer conn.Close()
		c := botrpc.NewBotFuncsClient(conn)

		// set the FuncName and send it to the bot.
		in.FuncName = cf.FuncName
		stream, err := c.SendMessage(context.Background(), in)
		for {
			// read response from bot
			in, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("error streaming from BotFuncs: %v", err)
				break
			}
			// send it to integration
			if err := outStream.Send(in); err == io.EOF {
				break
			} else if err != nil {
				log.Printf("error streaming to integration: %v", err)
				break
			}
		}
	}
	return nil
}
