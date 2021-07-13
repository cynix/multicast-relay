package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"

	"golang.org/x/net/ipv4"
	"golang.org/x/sys/unix"
)


type ListenConn interface {
	Close() error
	ReadPacket([]byte) ([]byte, PacketInfo, error)
}


func NewListenConn(ifs []*net.Interface, group Endpoint) (ListenConn, error) {
	// We need a raw socket to get the DSCP value
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_RAW, unix.IPPROTO_UDP);
	if err != nil {
		return nil, fmt.Errorf("failed to create socket: %w", err)
	}
	defer func() {
		if fd != -1 {
			unix.Close(fd)
		}
	}()

	if err = unix.SetNonblock(fd, true); err != nil {
		return nil, fmt.Errorf("failed to enable non-blocking: %w", err)
	}

	if err = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
		return nil, fmt.Errorf("failed to enable address reuse: %w", err)
	}

	if err = unix.Bind(fd, &unix.SockaddrInet4{Port: group.Port, Addr: group.Host}); err != nil {
		return nil, fmt.Errorf("failed to bind to %v: %w", group, err)
	}

	f := os.NewFile(uintptr(fd), "socket")
	fd = -1 // now owned by `f`
	defer f.Close()

	pc, err := net.FilePacketConn(f)
	if err != nil {
		return nil, fmt.Errorf("failed to create packet connection: %w", err)
	}
	defer func() {
		if pc != nil {
			pc.Close()
		}
	}()

	rc, err := ipv4.NewRawConn(pc)
	if err != nil {
		return nil, fmt.Errorf("failed to create raw connection: %w", err)
	}
	pc = nil // now owned by `rc`
	defer func() {
		if rc != nil {
			rc.Close()
		}
	}()

	if err = rc.SetMulticastLoopback(false); err != nil {
		return nil, fmt.Errorf("failed to disable multicast loopback: %w", err)
	}

	if err = rc.SetControlMessage(ipv4.FlagInterface | ipv4.FlagSrc | ipv4.FlagDst | ipv4.FlagTTL, true); err != nil {
		return nil, fmt.Errorf("failed to set control options: %w", err)
	}

	for _, ifi := range ifs {
		if err = rc.JoinGroup(ifi, &net.UDPAddr{IP: group.Host.IP(), Port: group.Port}); err != nil {
			return nil, fmt.Errorf("failed to join %v on %v: %w", group, ifi.Name, err)
		}
	}

	u := &udp4Conn{rc: rc}
	rc = nil // now owned by `u`
	return u, nil
}


type udp4Conn struct {
	rc *ipv4.RawConn
}

func (c *udp4Conn) Close() error {
	return c.rc.Close()
}

func (c *udp4Conn) ReadPacket(b []byte) (p []byte, pi PacketInfo, err error) {
	var h *ipv4.Header
	var cm *ipv4.ControlMessage

	h, p, cm, err = c.rc.ReadFrom(b)
	if err != nil {
		return
	}

	if len(p) < 8 {
		err = fmt.Errorf("invalid IP payload length: %d < 8", len(p))
		return
	}

	srcPort := int(binary.BigEndian.Uint16(p[0:2]))
	dstPort := int(binary.BigEndian.Uint16(p[2:4]))
	length  := int(binary.BigEndian.Uint16(p[4:6]))

	if len(p) < length {
		err = fmt.Errorf("invalid UDP payload length: %d < %d", len(p), length)
		return
	}

	pi.Src = Endpoint{Host: ToIP4(h.Src), Port: srcPort}
	pi.Dst = Endpoint{Host: ToIP4(h.Dst), Port: dstPort}
	pi.IfIndex = cm.IfIndex
	pi.DSCP = h.TOS >> 2
	pi.TTL = cm.TTL

	return
}
