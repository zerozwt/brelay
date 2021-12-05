package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"

	"github.com/zerozwt/brelay/client"
	"github.com/zerozwt/toyframe"
)

func getSecret(sub_id uint32) []byte {
	key := []byte(gConf.LoginKey)
	if len(key) == 0 {
		key = []byte(`brelay_key`)
	}

	data := make([]byte, len(key)+4)
	binary.Write(bytes.NewBuffer(data[:0]), binary.BigEndian, sub_id)
	copy(data[4:], key)

	sum := sha256.Sum224(data)
	key = sum[:16]
	nonce := sum[16:]

	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	return gcm.Seal(nil, nonce, data, nil)
}

func loginHandler(ctx *toyframe.Context) error {
	wgAll.Add(1)
	ctx.AddCloseHandler(wgAll.Done)
	ctx.SetInterruptor(gServer.CloseChannel())

	// get login msg
	login_req := client.MsgLoginReq{}
	if err := ctx.ReadObj(&login_req); err != nil {
		logger().Printf("read login request failed: %v", err)
		return err
	}

	mb := make(subMailbox)
	sub_id, err := gDanmaku.AllocSubscriberID(mb)

	if err != nil {
		return err // it has to be a ErrInterrupted
	} else {
		logger().Printf("client %s get an subscriber id %d", login_req.ID, sub_id)
	}
	defer gDanmaku.Logout(sub_id)

	// send login response
	login_rsp := client.MsgLoginRsp{
		SubscriberID:     sub_id,
		SubscriberSecret: getSecret(sub_id),
	}
	if err = ctx.WriteObj(&login_rsp); err != nil {
		logger().Printf("send login reponse failed: %v", err)
		return err
	}

	// send batch data
	for batch := range mb {
		if err = ctx.WriteObj(&batch); err != nil {
			logger().Printf("send batch msgs to client %s failed: %v", login_req.ID, err)
			return err
		}
	}
	return nil
}

func logoutHandler(ctx *toyframe.Context) error {
	wgAll.Add(1)
	ctx.AddCloseHandler(wgAll.Done)
	ctx.SetInterruptor(gServer.CloseChannel())

	// get logout msg
	logout_req := client.MsgLogoutReq{}
	if err := ctx.ReadObj(&logout_req); err != nil {
		logger().Printf("read logout request failed: %v", err)
		return err
	}

	rsp := client.MsgLogoutRsp{}
	rsp.Ok = true

	sec := getSecret(logout_req.SubscriberID)
	if !bytes.Equal(sec, logout_req.SubscriberSecret) {
		logger().Printf("logout failed: secret check failed")
		rsp.Ok = false
		rsp.Msg = "logout failed: secret check failed"
	}

	if rsp.Ok {
		gDanmaku.Logout(logout_req.SubscriberID)
	}

	if err := ctx.WriteObj(&rsp); err != nil {
		logger().Printf("send logout reponse failed: %v", err)
	}
	return nil
}

func subscribeHandler(ctx *toyframe.Context) error {
	wgAll.Add(1)
	ctx.AddCloseHandler(wgAll.Done)
	ctx.SetInterruptor(gServer.CloseChannel())

	// get subscribe msg
	req := client.MsgSubscribeReq{}
	if err := ctx.ReadObj(&req); err != nil {
		logger().Printf("read subscribe request failed: %v", err)
		return err
	}

	// check secret
	sec := getSecret(req.SubscriberID)
	if !bytes.Equal(sec, req.SubscriberSecret) {
		logger().Printf("subscribe failed: secret check failed")
		ctx.WriteObj(&client.MsgSubscribeRsp{Ok: false, Msg: "secret check failed"})
		return nil
	}

	// clear current subscriptions
	if gDanmaku.ClearSubscribe(req.SubscriberID) == nil {
		for _, sub := range req.Rooms {
			// subscribe
			gDanmaku.SubscribeRoom(sub.RoomID, req.SubscriberID, sub.Cmds)
		}
	}

	if err := ctx.WriteObj(&client.MsgSubscribeRsp{Ok: true}); err != nil {
		logger().Printf("send subscribe reponse failed: %v", err)
	}
	return nil
}
