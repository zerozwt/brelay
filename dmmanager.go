package main

import (
	"crypto/rand"
	"encoding/binary"
	math_rand "math/rand"
	"sync/atomic"
	"time"

	jsoniter "github.com/json-iterator/go"
	dm "github.com/zerozwt/BLiveDanmaku"
	"github.com/zerozwt/brelay/client"
	"github.com/zerozwt/toyframe"
)

type subscriberInfo struct {
	id   uint32
	cmds []string
}

type subMailbox chan client.MsgSubscribeBatch

type dmManager struct {
	job_ch  chan func()
	subs    map[int]map[uint32]*subscriberInfo    // room_id => subscriber_id => subscribers
	cache   map[uint32][]client.MsgSubscribeBatch // subscriber_id => cached msgs
	mailbox map[uint32]subMailbox                 // subscriber_id => recv channel
	next_id uint32
}

var gDanmaku *dmManager = newBLiveDanmakuManager()

func newBLiveDanmakuManager() *dmManager {
	ret := &dmManager{
		job_ch:  make(chan func(), 1024),
		subs:    make(map[int]map[uint32]*subscriberInfo),
		cache:   make(map[uint32][]client.MsgSubscribeBatch),
		mailbox: make(map[uint32]subMailbox),
	}
	if err := binary.Read(rand.Reader, binary.BigEndian, &(ret.next_id)); err != nil {
		logger().Printf("subscriber id generator from crypto/rand failed: %v, use math/rand instead", err)
		ret.next_id = math_rand.New(math_rand.NewSource(time.Now().Unix())).Uint32()
	}
	go ret.flushSchedule()
	go ret.run()
	return ret
}

func (m *dmManager) PostJob(job func()) {
	m.job_ch <- job
}

func (m *dmManager) ExecJob(job func()) error {
	return toyframe.DoWithInterruptor(func() {
		done_ch := make(chan struct{})
		m.PostJob(func() {
			defer close(done_ch)
			job()
		})
		<-done_ch
	}, gServer.CloseChannel())
}

func (m *dmManager) flushSchedule() {
	for m.ExecJob(m.flushData) == nil {
		time.Sleep(time.Second)
	}
}

func (m *dmManager) run() {
	for {
		select {
		case job := <-m.job_ch:
			m.safeRun(job)
		case <-gServer.CloseChannel():
			m.onServerShutdown()
			return
		}
	}
}

func (m *dmManager) safeRun(job func()) {
	defer func() {
		if err := recover(); err != nil {
			logger().Printf("danmaku manger job panic: %v", err)
		}
	}()
	job()
}

func (m *dmManager) flushData() {
	new_cache := make(map[uint32][]client.MsgSubscribeBatch)
	for sub_id, cache := range m.cache {
		batch := client.MsgSubscribeBatch{}
		for _, item := range cache {
			batch.CombineWith(&item)
		}
		if len(batch.Msgs) == 0 {
			continue
		}

		if mailbox, ok := m.mailbox[sub_id]; ok {
			select {
			case mailbox <- batch:
				continue // batch successfully send to mailbox
			default:
				new_cache[sub_id] = append(new_cache[sub_id], batch)
			}
		}
		// drop cached msgs if subscriber not exist
	}
	m.cache = new_cache
}

func (m *dmManager) onServerShutdown() {
	go func() {
		for range m.job_ch {
			// clear all remain jobs but do nothing
		}
	}()

	// clear cache
	m.cache = make(map[uint32][]client.MsgSubscribeBatch)

	// clear all subscribers
	m.subs = make(map[int]map[uint32]*subscriberInfo)

	// close all mailbox
	for _, mailbox := range m.mailbox {
		close(mailbox)
	}
	m.mailbox = make(map[uint32]subMailbox)
}

func (m *dmManager) AllocSubscriberID(mailbox subMailbox) (ret uint32, err error) {
	err = m.ExecJob(func() {
		ret = atomic.AddUint32(&(m.next_id), 1)
		m.mailbox[ret] = mailbox
	})
	return
}

func (m *dmManager) ClearSubscribe(sub_id uint32) error {
	return m.ExecJob(func() { m.clearSubBySubID(sub_id) })
}

func (m *dmManager) clearSubBySubID(sub_id uint32) {
	new_subs := make(map[int]map[uint32]*subscriberInfo)
	for room_id, room_sub := range m.subs {
		if _, ok := room_sub[sub_id]; ok {
			delete(room_sub, sub_id)
			if len(room_sub) > 0 {
				new_subs[room_id] = room_sub
			}
		} else {
			new_subs[room_id] = room_sub
		}
	}
	m.subs = new_subs
}

func (m *dmManager) SubscribeRoom(room_id int, sub_id uint32, cmds []string) {
	m.PostJob(func() {
		info := &subscriberInfo{
			id:   sub_id,
			cmds: append([]string{}, cmds...),
		}
		if _, ok := m.subs[room_id]; !ok {
			m.subs[room_id] = make(map[uint32]*subscriberInfo)
		}
		m.subs[room_id][sub_id] = info

		go gClientMgr.AddClient(sub_id, room_id)
	})
}

func (m *dmManager) Logout(sub_id uint32) {
	m.PostJob(func() {
		m.clearSubBySubID(sub_id)

		batch := client.MsgSubscribeBatch{}
		if cache, ok := m.cache[sub_id]; ok {
			delete(m.cache, sub_id)
			for _, item := range cache {
				batch.CombineWith(&item)
			}
		}

		if mailbox, ok := m.mailbox[sub_id]; ok {
			if len(batch.Msgs) > 0 {
				select {
				case mailbox <- batch:
					func() {}() // noop
				default:
					func() {}() // noop
				}
			}
			delete(m.mailbox, sub_id)
			close(mailbox)
		}
	})
}

func (m *dmManager) UpdateRommState(room_id int, msg_type int, room_info *dm.RoomInfo, sub_id_list []uint32) {
	batch := client.MsgSubscribeBatch{
		Msgs: []client.MsgSubscribeData{
			{
				RoomID:  room_id,
				MsgType: byte(msg_type),
			},
		},
	}
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	if room_info != nil {
		batch.Msgs[0].Data, _ = json.Marshal(room_info)
	}

	m.PostJob(func() {
		if len(sub_id_list) == 0 {
			if sub_list, ok := m.subs[room_id]; ok {
				for _, item := range sub_list {
					sub_id_list = append(sub_id_list, item.id)
				}
			}
		}
		for _, id := range sub_id_list {
			m.cache[id] = append(m.cache[id], batch.Clone())
		}
	})
}

func (m *dmManager) OnRoomMsg(room_id int, cmd string, data []byte) {
	batch := client.MsgSubscribeBatch{
		Msgs: []client.MsgSubscribeData{
			{
				RoomID:  room_id,
				MsgType: client.MSG_TYPE_DATA,
				Cmd:     cmd,
				Data:    data,
			},
		},
	}

	m.PostJob(func() {
		if sub_list, ok := m.subs[room_id]; ok {
			for _, item := range sub_list {
				for _, tmp := range item.cmds {
					if tmp == cmd {
						m.cache[item.id] = append(m.cache[item.id], batch.Clone())
					}
				}
			}
		}
	})
}

func (m *dmManager) OnRoomLiveStateChange(room_id int, cmd string, data []byte) {
	batch := client.MsgSubscribeBatch{
		Msgs: []client.MsgSubscribeData{
			{
				RoomID:  room_id,
				MsgType: client.MSG_TYPE_DATA,
				Cmd:     cmd,
				Data:    data,
			},
		},
	}

	m.PostJob(func() {
		if sub_list, ok := m.subs[room_id]; ok {
			for _, item := range sub_list {
				m.cache[item.id] = append(m.cache[item.id], batch.Clone())
			}
		}
	})
}
