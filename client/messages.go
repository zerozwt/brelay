package client

//go:generate msgp

type MsgLoginReq struct {
	ID string `msg:"id"`
}

type MsgLoginRsp struct {
	SubscriberID     uint32 `msg:"sid"`
	SubscriberSecret []byte `msg:"sec"`
}

//------------------------------------------------------------------

type MsgLogoutReq struct {
	SubscriberID     uint32 `msg:"sid"`
	SubscriberSecret []byte `msg:"sec"`
}

type MsgLogoutRsp struct {
	Ok  bool   `msg:"ok"`
	Msg string `msg:"msg"`
}

//------------------------------------------------------------------

type MsgSubscribeReq struct {
	SubscriberID     uint32             `msg:"sid"`
	SubscriberSecret []byte             `msg:"sec"`
	Rooms            []MsgSubscribeRoom `msg:"rooms"`
}

type MsgSubscribeRoom struct {
	RoomID int      `msg:"room"`
	Cmds   []string `msg:"cmds"`
}

type MsgSubscribeRsp struct {
	Ok  bool   `msg:"ok"`
	Msg string `msg:"msg"`
}

//------------------------------------------------------------------

type MsgSubscribeBatch struct {
	Msgs []MsgSubscribeData `msg:"msgs"`
}

type MsgSubscribeData struct {
	RoomID  int    `msg:"room"`
	MsgType byte   `msg:"type"`
	Cmd     string `msg:"cmd"`
	Data    []byte `msg:"data"`
}

func (b *MsgSubscribeBatch) CombineWith(other *MsgSubscribeBatch) {
	b.Msgs = append(b.Msgs, other.Msgs...)
}

func (b *MsgSubscribeBatch) Clone() MsgSubscribeBatch {
	return MsgSubscribeBatch{Msgs: append([]MsgSubscribeData{}, b.Msgs...)}
}

const (
	MSG_TYPE_DATA           = 0
	MSG_TYPE_WS_CONNECT     = 1
	MSG_TYPE_WS_DISCONNECT  = 2
	MSG_TYPE_ROOM_CONN_FAIL = 3
)
