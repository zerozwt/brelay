package main

import (
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	dm "github.com/zerozwt/BLiveDanmaku"
	"github.com/zerozwt/brelay/client"
	"github.com/zerozwt/toyframe/dialer"
)

var test_init int32
var test_dial dialer.DialFunc = dialer.B(net.Dial).WithBitReverse().WithMultiplex().WithBrotli().Build()

func testInit(t *testing.T) bool {
	if !atomic.CompareAndSwapInt32(&test_init, 0, 1) {
		return true
	}

	gConf.Inbounds = []InboundConfig{
		{
			Name:    "test",
			Addr:    "localhost:6789",
			Filters: []string{"reverse", "multiplex", "brotli"},
		},
	}

	if !initInbounds() {
		t.Errorf("init inbound error")
		return false
	}
	gServer.Register("login", loginHandler)
	gServer.Register("logout", logoutHandler)
	gServer.Register("subscribe", subscribeHandler)

	go gServer.Run()
	return true
}

func testClient(name string, life time.Duration, t *testing.T) {
	relay_client := client.NewBRelayClient(name, "tcp", "localhost:6789", test_dial, nil)
	ctx, err := relay_client.Login()
	if err != nil {
		t.Errorf("client login failed: %v", err)
		return
	}
	defer ctx.Close()

	err = relay_client.Subscribe([]client.MsgSubscribeRoom{{RoomID: 22865391, Cmds: []string{dm.CMD_DANMU_MSG}}})
	if err != nil {
		t.Errorf("client subscribe failed: %v", err)
		return
	}

	go func() {
		time.Sleep(life)
		if err := relay_client.Logout(); err != nil {
			t.Errorf("client logout failed: %v", err)
			return
		}
	}()

	for {
		arr, err := relay_client.ReadMessages(ctx)
		if err == io.EOF {
			logger().Printf("client %s meet EOF", name)
			ctx.Close()
			break
		}
		if err != nil {
			t.Errorf("read messages failed: %v", err)
			return
		}
		logger().Printf("[%s]recieved %d messages", name, len(arr))
		for _, item := range arr {
			data := item.Data
			if item.Cmd == dm.CMD_DANMU_MSG {
				data = relay_client.ReadBodyBytes(data, "info")
			}
			logger().Printf("[%s]  cmd=%s msg_type=%d data=%s", name, item.Cmd, item.MsgType, string(data))
		}
	}
}

func TestAll(t *testing.T) {
	if !testInit(t) {
		return
	}

	// server shutdown after 10 seconds
	go func() {
		time.Sleep(time.Second * 10)
		gServer.Close()
	}()

	go testClient("test1", 5*time.Second, t)
	go testClient("test2", 15*time.Second, t)
	time.Sleep(5 * time.Second)
	go testClient("test3", 5*time.Second, t)
	wgAll.Wait()
}
