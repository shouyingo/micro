package main

import (
	"log"
	"strconv"
	"time"

	"github.com/shouyingo/consul"
	"github.com/shouyingo/micro"
)

func ondone(method string, code int, result []byte) {
	t, _ := strconv.ParseInt(string(result), 10, 64)
	log.Println("done", time.Duration(time.Now().UnixNano()-t))
}

func main() {
	c := micro.NewClient(consul.New("http://127.0.0.1:8500", ""), []string{"demosvc"})
	c.OnError = func(action string, err error) {
		log.Println("err", action, err)
	}
	for {
		err := c.Call("demosvc", "hello", strconv.AppendInt(nil, time.Now().UnixNano(), 10), ondone)
		if err != nil {
			log.Println("call", err)
		}
		time.Sleep(time.Second)
	}
}
