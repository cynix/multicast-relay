package main

import (
	"net"

	"github.com/valyala/bytebufferpool"
)


type Packet struct {
	Buffer *bytebufferpool.ByteBuffer
	Info PacketInfo
	Payload []byte
}

type Listener struct {
	e Endpoint
	c ListenConn
	dscp int
	ifs map[int]struct{}
	ch chan<- interface{}
}

func NewListener(ifs []*net.Interface, group Endpoint, dscp int, ch chan<- interface{}) (*Listener, error) {
	c, err := NewListenConn(ifs, group)
	if err != nil {
		return nil, err
	}

	l := &Listener{c: c, dscp: dscp, ifs: make(map[int]struct{}), ch: ch}

	for _, i := range ifs {
		l.ifs[i.Index] = struct{}{}
	}

	return l, nil
}

func (l *Listener) Listen() {
	defer l.c.Close()

	var err error

	for {
		p := Packet{Buffer: bytebufferpool.Get()}
		if len(p.Buffer.B) == 0 {
			p.Buffer.B = make([]byte, 1536)
		}

		if err = l.read(&p); err != nil {
			l.ch <- err
		}

		if p.Payload != nil {
			l.ch <- p
		} else {
			bytebufferpool.Put(p.Buffer)
		}
	}
}

func (l *Listener) read(p *Packet) error {
	var payload []byte
	var err error

	if payload, p.Info, err = l.c.ReadPacket(p.Buffer.B); err != nil {
		return err
	}

	if p.Info.DSCP == l.dscp {
		// Loopback of our own packet
		return nil
	}

	if _, ok := l.ifs[p.Info.IfIndex]; !ok {
		// Packet not intended for our interfaces
		return nil
	}

	p.Payload = payload
	return nil
}
