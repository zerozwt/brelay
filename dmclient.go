package main

import (
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"
	dm "github.com/zerozwt/BLiveDanmaku"
	"github.com/zerozwt/brelay/client"
	"github.com/zerozwt/toyframe"
)

type dmClientManager struct {
	sync.Mutex
	clients map[int]*dmClient
}

const (
	DM_CLIENT_STATE_INIT       = 0
	DM_CLIENT_STATE_CONNECTING = 1
	DM_CLIENT_STATE_CONNECTED  = 2
)

type dmClient struct {
	client *dm.Client
	state  int
}

var gClientMgr *dmClientManager = &dmClientManager{
	clients: make(map[int]*dmClient),
}

func (m *dmClientManager) AddClient(sub_id uint32, room_id int) {
	m.Lock()
	defer m.Unlock()

	if info, ok := m.clients[room_id]; ok {
		if info.state == DM_CLIENT_STATE_CONNECTED {
			gDanmaku.UpdateRommState(room_id, client.MSG_TYPE_WS_CONNECT, info.client.Room(), []uint32{sub_id})
		}
		return
	}

	// mark as connecting
	m.clients[room_id] = &dmClient{
		client: nil,
		state:  DM_CLIENT_STATE_CONNECTING,
	}

	// async dial
	go func() {
		tmp, err := dm.Dial(room_id, m.dmConfig(room_id))
		m.onDialResult(room_id, tmp, err)
	}()
}

func (m *dmClientManager) onDialResult(room_id int, dm_client *dm.Client, err error) {
	m.Lock()
	defer m.Unlock()

	if err != nil {
		delete(m.clients, room_id)
		gDanmaku.UpdateRommState(room_id, client.MSG_TYPE_ROOM_CONN_FAIL, nil, nil)
		return
	}

	m.clients[room_id] = &dmClient{
		client: dm_client,
		state:  DM_CLIENT_STATE_CONNECTED,
	}
	gDanmaku.UpdateRommState(room_id, client.MSG_TYPE_WS_CONNECT, dm_client.Room(), nil)
}

func (m *dmClientManager) dmConfig(room_id int) *dm.ClientConf {
	tmp_disconnect := func(dm_client *dm.Client, err error) {
		m.onDisconnect(room_id, dm_client, err)
	}
	ret := &dm.ClientConf{
		OnNetError:         tmp_disconnect,
		OnServerDisconnect: tmp_disconnect,
	}
	ret.AddOpHandler(dm.OP_SEND_MSG_REPLY, func(_ *dm.Client, msg *dm.RawMessage) bool {
		return m.onRoomMsg(room_id, msg)
	})
	tmp_live_state_change := func(dm_client *dm.Client, cmd string, _ []byte) bool {
		return m.onLiveStateChange(dm_client, cmd, room_id)
	}
	ret.AddCmdHandler(dm.CMD_LIVE, tmp_live_state_change)
	ret.AddCmdHandler(dm.CMD_PREPARING, tmp_live_state_change)
	return ret
}

func (m *dmClientManager) onRoomMsg(room_id int, msg *dm.RawMessage) bool {
	iter := jsoniter.NewIterator(jsoniter.ConfigCompatibleWithStandardLibrary).ResetBytes(msg.Data)
	cmd := ""
	iter.ReadObjectCB(func(iter *jsoniter.Iterator, key string) bool {
		if key == "cmd" {
			cmd = iter.ReadString()
		} else {
			iter.Skip()
		}
		return true
	})
	if cmd == dm.CMD_LIVE || cmd == dm.CMD_PREPARING {
		return false
	}
	gDanmaku.OnRoomMsg(room_id, cmd, msg.Data)
	return false
}

func (m *dmClientManager) onLiveStateChange(dm_client *dm.Client, cmd string, room_id int) bool {
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	data, _ := json.Marshal(dm_client.Room())
	gDanmaku.OnRoomLiveStateChange(room_id, cmd, data)
	return false
}

func (m *dmClientManager) onDisconnect(room_id int, dm_client *dm.Client, err error) {
	m.Lock()
	defer m.Unlock()

	// notify subscribers
	logger().Printf("coneection to live_room %d interrupted: %v", room_id, err)
	gDanmaku.UpdateRommState(room_id, client.MSG_TYPE_WS_DISCONNECT, dm_client.Room(), nil)

	// mark as connecting before reconnect
	m.clients[room_id] = &dmClient{
		client: nil,
		state:  DM_CLIENT_STATE_CONNECTING,
	}

	// try reconnect ...
	go func() {
		logger().Printf("try reconnect live room %d ...", room_id)

		wait_time := time.Second
		max_wait_time := 30 * time.Second

		for {
			err2 := toyframe.DoWithInterruptor(func() {
				dm_client, err = dm.Dial(room_id, m.dmConfig(room_id))
			}, gServer.CloseChannel())

			if err2 != nil {
				logger().Printf("reconnect live room %d failed: %v", room_id, err)
				return
			}

			if err == nil {
				m.onDialResult(room_id, dm_client, err)
				return
			}

			logger().Printf("reconnect live room %d failed: %v, reconnect after %v ...", room_id, err, wait_time)
			time.Sleep(wait_time)
			wait_time *= 2
			if wait_time > max_wait_time {
				wait_time = max_wait_time
			}
		}
	}()
}
