package main

import (
	"encoding/binary"
	"fmt"
	"net"
)


type IP4 [4]byte

type Endpoint struct {
	Host IP4
	Port int // in host byte order
}

type PacketInfo struct {
	Src Endpoint
	Dst Endpoint
	IfIndex int
	DSCP int
	TTL int
}


func ToIP4(a net.IP) IP4 {
	var h IP4
	copy(h[:], a.To4())
	return h
}

func (h IP4) IP() net.IP {
	return net.IPv4(h[0], h[1], h[2], h[3])
}

func (e Endpoint) String() string {
	return fmt.Sprintf("%d.%d.%d.%d:%d", e.Host[0], e.Host[1], e.Host[2], e.Host[3], e.Port)
}

func (e Endpoint) Key() uint64 {
	return uint64(binary.BigEndian.Uint32(e.Host[:])) << 16 | uint64(e.Port)
}
