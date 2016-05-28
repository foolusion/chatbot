package main

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/net/context"

	"google.golang.org/grpc"

	"github.com/foolusion/chatbot/botrpc"
)

const address = "localhost:8173"
const port = ":8081"

type server struct{}

func (s *server) SendMessage(in *botrpc.ChatMessage, stream botrpc.BotFuncs_SendMessageServer) error {
	switch in.FuncName {
	case "hello":
		hello(in, stream)
	default:
		return fmt.Errorf("func does not exist: %v", *in)
	}
	return nil
}

func main() {
	if err := register(); err != nil {
		fmt.Fprintf(os.Stdout, "error registering hellobot: %v", err)
	}

	lis, err := net.Listen("tcp", port)
	if err != nil {
		fmt.Fprintf(os.Stdout, "failed to listen: %v\n", err)
	}
	s := grpc.NewServer()
	botrpc.RegisterBotFuncsServer(s, &server{})
	s.Serve(lis)
}

func register() error {
	addr, err := getIP()
	if err != nil {
		return err
	}
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		fmt.Fprintf(os.Stdout, "error connecting with client: %v", err)
	}
	defer conn.Close()
	cfd := &botrpc.Func{
		Addr:     addr + port,
		Trigger:  "hello",
		FuncName: "hello",
		Usage:    "bot responds when you say \"hello\".",
	}
	c := botrpc.NewBotClient(conn)
	// eventually do something with fs
	_, err = c.Add(context.Background(), cfd)
	if err != nil {
		return err
	}
	return nil
}

func hello(in *botrpc.ChatMessage, stream botrpc.BotFuncs_SendMessageServer) {
	in.Body = "hey there"
	stream.Send(in)
}

func getIP() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			return "", err
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip.IsLoopback() {
				continue
			}
			return ip.String(), nil
		}
	}
	return "", fmt.Errorf("No valid interfaces found")
}
