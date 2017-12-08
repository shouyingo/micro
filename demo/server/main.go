package main

import (
	"log"

	"github.com/shouyingo/consul"
	"github.com/shouyingo/micro"
)

func oncall(c *micro.Context) {
	log.Println(c.Method())
	c.Reply(0, c.Params())
}

func main() {
	s := micro.NewService(consul.New("http://127.0.0.1:8500", ""), "demosvc", "127.0.0.1", oncall)
	s.Start()
}
