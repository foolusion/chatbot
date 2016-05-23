package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"

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

const port = ":8080"

func main() {
	// connect to chat

	// listen for incoming chat
	go listen(os.Stdin)

	// start registration server
	lis, err := net.Listen("tcp", port)
	if err != nil {
		fmt.Fprintf(os.Stdout, "failed to listen: %v\n", err)
	}
	s := grpc.NewServer()
	botrpc.RegisterBotServer(s, &server{})
	s.Serve(lis)
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

func handleChat(msg string) {
	if chatFuncs == nil {
		return
	}
	for _, cf := range chatFuncs {
		if ok := cf.triggerExpr.MatchString(msg); !ok {
			continue
		}
		resp, err := http.Get(cf.Port)
		if err != nil {
			fmt.Fprintf(os.Stdout, "response from chatFunc: %v\n", err)
		} else if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stdout, "non ok response from chatFunc: %v\n", resp.Status)
		}
		// write resp to chat conn
		io.Copy(os.Stdout, resp.Body)
	}
}
