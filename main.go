package main

import (
	"crypto/tls"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/zerozwt/toyframe"
	"github.com/zerozwt/toyframe/listener"
)

var gServer toyframe.Server = toyframe.NewServer()
var wgAll sync.WaitGroup

func initInbounds() bool {
	for _, item := range gConf.Inbounds {
		lis, err := net.Listen("tcp", item.Addr)
		if err != nil {
			logger().Printf("listen on [%s](%s) failed: %v", item.Name, item.Addr, err)
			return false
		}
		if len(item.Filters) == 0 {
			gServer.AddListener(lis)
			continue
		}
		builder := listener.B(lis)
		for _, filter := range item.Filters {
			switch filter {
			case "reverse":
				builder = builder.WithBitReverse()
			case "brotli":
				builder = builder.WithBrotli()
			case "multiplex":
				builder = builder.WithMultiplex()
			case "tls":
				cert, err := tls.LoadX509KeyPair(item.TlsPEMFile, item.TlsKeyFile)
				if err != nil {
					logger().Printf("create tls filter for listener %s failed: %v", item.Name, err)
					return false
				}
				builder = builder.WithTls(&tls.Config{Certificates: []tls.Certificate{cert}})
			default:
				logger().Printf("unknown filter %s in listener %s", filter, item.Name)
				return false
			}
		}
		gServer.AddListener(builder.Build())
	}
	return true
}

func main() {
	// load config
	if !initConf() {
		return
	}

	// init log
	SetLogWriter(dailyFileLogWriter(gConf.LogFile))

	// build listeners
	if len(gConf.Inbounds) == 0 {
		logger().Printf("no inbounds specified!")
		return
	}
	if !initInbounds() {
		return
	}

	logger().Printf("bilibili live danmaku relay server start.....")

	go func() {
		tmp := make(chan os.Signal, 1)
		signal.Notify(tmp, syscall.SIGINT, syscall.SIGTERM)
		<-tmp
		logger().Printf("exit signal recieved, server shutdown ...")
		gServer.Close()
	}()

	// register handlers
	gServer.Register("login", loginHandler)
	gServer.Register("logout", logoutHandler)
	gServer.Register("subscribe", subscribeHandler)

	// start server
	gServer.Run()
	wgAll.Wait()
	logger().Printf("bilibili live danmaku relay server has been shutdown")
}
