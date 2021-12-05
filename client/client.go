package client

import (
	"errors"

	jsoniter "github.com/json-iterator/go"
	"github.com/zerozwt/toyframe"
	"github.com/zerozwt/toyframe/dialer"
)

type Client struct {
	id   uint32
	name string
	sec  []byte

	network string
	address string
	dial    dialer.DialFunc

	ich chan struct{}
}

func NewBRelayClient(name, network, address string, dial dialer.DialFunc, ich chan struct{}) *Client {
	return &Client{
		name:    name,
		network: network,
		address: address,
		dial:    dial,
		ich:     ich,
	}
}

func (c *Client) Login() (*toyframe.Context, error) {
	ctx, err := toyframe.CallWithInterruptor(c.network, c.address, "login", c.dial, c.ich, &MsgLoginReq{ID: c.name})
	if err != nil {
		return nil, err
	}

	rsp := MsgLoginRsp{}
	err = ctx.ReadObj(&rsp)
	if err != nil {
		ctx.Close()
		return nil, err
	}

	c.id = rsp.SubscriberID
	c.sec = rsp.SubscriberSecret
	return ctx, nil
}

func (c *Client) Logout() error {
	ctx, err := toyframe.CallWithInterruptor(c.network, c.address, "logout", c.dial, c.ich,
		&MsgLogoutReq{SubscriberID: c.id, SubscriberSecret: c.sec})
	if err != nil {
		return err
	}
	defer ctx.Close()

	rsp := MsgLogoutRsp{}
	err = ctx.ReadObj(&rsp)
	if err != nil {
		return err
	}

	if len(rsp.Msg) > 0 {
		return errors.New(rsp.Msg)
	}
	return nil
}

func (c *Client) Subscribe(rooms []MsgSubscribeRoom) error {
	ctx, err := toyframe.CallWithInterruptor(c.network, c.address, "subscribe", c.dial, c.ich,
		&MsgSubscribeReq{SubscriberID: c.id, SubscriberSecret: c.sec, Rooms: rooms})
	if err != nil {
		return err
	}
	defer ctx.Close()

	rsp := MsgSubscribeRsp{}
	err = ctx.ReadObj(&rsp)
	if err != nil {
		return err
	}

	if len(rsp.Msg) > 0 {
		return errors.New(rsp.Msg)
	}
	return nil
}

func (c *Client) ReadMessages(ctx *toyframe.Context) ([]MsgSubscribeData, error) {
	batch := MsgSubscribeBatch{}
	if err := ctx.ReadObj(&batch); err != nil {
		return nil, err
	}
	return batch.Msgs, nil
}

func (c *Client) ReadBodyBytes(data []byte, body_key string) []byte {
	ret := []byte{}
	iter := jsoniter.NewIterator(jsoniter.ConfigCompatibleWithStandardLibrary).ResetBytes(data)
	iter.ReadObjectCB(func(iter *jsoniter.Iterator, key string) bool {
		if key == body_key {
			ret = iter.SkipAndReturnBytes()
		} else {
			iter.Skip()
		}
		return true
	})
	return ret
}
