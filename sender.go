package main

import (
	"net"
	"sync"
)


type ConnPool map[int]SendConn

type Sender struct {
	cs []SendConn
	ch chan error
	wg sync.WaitGroup
}

func NewConnPool() ConnPool {
	return make(map[int]SendConn)
}

func NewSender(ifs []*net.Interface, pool ConnPool) (*Sender, error) {
	var err error

	cs := make([]SendConn, 0, len(ifs))

	for _, ifi := range ifs {
		c, ok := pool[ifi.Index]
		if !ok {
			c, err = NewSendConn(ifi)
			if err != nil {
				return nil, err
			}

			pool[ifi.Index] = c
		}

		cs = append(cs, c)
	}

	return &Sender{cs: cs, ch: make(chan error)}, nil
}

func (s *Sender) Send(pi PacketInfo, b []byte) []error {
	s.wg.Add(len(s.cs))

	for _, c := range s.cs {
		go func(c SendConn) {
			defer s.wg.Done()

			if err := c.WritePacket(pi, b); err != nil {
				s.ch <- err
				return
			}
		}(c)
	}

	s.wg.Wait()

	var errs []error

_for:
	for {
		select {
		case err := <- s.ch:
			errs = append(errs, err)
		default:
			break _for
		}
	}

	return errs
}
