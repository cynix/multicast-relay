package main

import (
	"encoding/binary"
	"fmt"
	"net"

	"golang.org/x/sys/unix"

	"github.com/mdlayher/raw"
)


type SendConn interface {
	Close() error
	IfIndex() int
	WritePacket(PacketInfo, []byte) error
}

func NewSendConn(ifi *net.Interface) (SendConn, error) {
	rc, err := raw.ListenPacket(ifi, ethertypeIPv4, nil)
	if err != nil {
		return nil, err
	}

	c := &rawConn{Conn: rc, ifIndex: ifi.Index}

	c.l2 = c.buf[:ethernetHeaderLen]
	c.l2[0] = 0x01 // dst (always 01:00:5e:... for multicast)
	c.l2[1] = 0x00
	c.l2[2] = 0x5e
	copy(c.l2[6:12], ifi.HardwareAddr) // src
	binary.BigEndian.PutUint16(c.l2[12:14], ethertypeIPv4) // ethertype

	c.l3 = c.buf[len(c.l2):len(c.l2)+ipv4HeaderLen]
	c.l3[0] = (4 << 4) | 5 // version | ihl
	c.l3[6] = 1 << 6 // flags: DF
	c.l3[9] = byte(unix.IPPROTO_UDP) // protocol

	c.l4 = c.buf[len(c.l2)+len(c.l3):]

	c.dst.HardwareAddr = c.l2[0:6]

	return c, nil
}


type rawConn struct {
	*raw.Conn
	ifIndex int
	buf [1536]byte
	l2 []byte
	l3 []byte
	l4 []byte
	dst raw.Addr
}

func (c *rawConn) IfIndex() int {
	return c.ifIndex
}

func (c *rawConn) WritePacket(pi PacketInfo, b []byte) error {
	if pi.IfIndex == c.ifIndex {
		// Packet arrived on the same interface
		return nil
	}

	c.l2[3] = pi.Dst.Host[1] & 0x7f // dst (lowest 23 bits of IP address)
	c.l2[4] = pi.Dst.Host[2]
	c.l2[5] = pi.Dst.Host[3]

	c.l3[1] = byte(pi.DSCP << 2) // tos
	binary.BigEndian.PutUint16(c.l3[2:4], uint16(ipv4HeaderLen + len(b))) // tot_len
	c.l3[8] = byte(pi.TTL) // ttl
	c.l3[10] = 0 // checksum
	c.l3[11] = 0
	copy(c.l3[12:16], pi.Src.Host[:]) // src
	copy(c.l3[16:20], pi.Dst.Host[:]) // dst
	binary.BigEndian.PutUint16(c.l3[10:12], uint16(checksum(c.l3))) // checksum

	copy(c.l4, b)

	p := c.buf[:ethernetHeaderLen+ipv4HeaderLen+len(b)]
	n, err := c.WriteTo(p, &c.dst)
	if err == nil && n < ethernetHeaderLen + ipv4HeaderLen + len(b) {
		err = fmt.Errorf("short write: if=%d dst=%v len=%d < %d", c.ifIndex, pi.Dst, n, len(p))
	}

	return err
}


const ethertypeIPv4 = 0x800

const (
	ethernetHeaderLen = 14
	ipv4HeaderLen = 20
)

func checksum(b []byte) uint16 {
	var sum uint32

	for i := 0; i < ipv4HeaderLen; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(b[i:i+2]))
	}

	for sum > 0xffff {
		sum = (sum & 0xffff) + (sum >> 16)
	}

	return uint16(^sum)
}
