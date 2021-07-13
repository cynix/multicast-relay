package main

import (
	"flag"
	"log"
	"net"
	"os"
	"strings"

	"github.com/valyala/bytebufferpool"
)


func main() {
	os.Exit(run())
}

func run() int {
	var id int
	var verbose bool

	flag.IntVar(&id, "id", 42, "unique identifier between 1 and 63 (inclusive)")
	flag.BoolVar(&verbose, "verbose", false, "verbose logging")
	flag.Parse()

	if id < 1 || id > 63 {
		log.Printf("invalid id: %v", id)
		return 1
	}

	if len(flag.Args()) == 0 {
		log.Printf("no relays specified")
		return 1
	}

	ifm, err := interfaces()
	if err != nil {
		log.Printf("failed to get network interfaces: %v", err)
		return 1
	}

	ch := make(chan interface{}, 512)
	listeners := make(map[uint64]*Listener)
	pool := NewConnPool()
	senders := make(map[uint64]*Sender)

	for _, arg := range flag.Args() {
		parts := strings.Split(arg, ",")
		if len(parts) < 3 {
			log.Printf("invalid relay specification: %v", arg)
			return 1
		}

		var group Endpoint

		switch parts[0] {
		case "mdns":
			group = Endpoint{Host: ToIP4(net.IPv4(224, 0, 0, 251)), Port: 5353}

		case "ssdp":
			group = Endpoint{Host: ToIP4(net.IPv4(239, 255, 255, 250)), Port: 1900}

		default:
			addr, err := net.ResolveUDPAddr("udp4", parts[0])
			if err != nil {
				log.Printf("invalid UDP address: %v: %v", parts[0], err)
				return 1
			}

			group = Endpoint{Host: ToIP4(addr.IP), Port: addr.Port}
		}

		if _, ok := listeners[group.Key()]; ok {
			log.Printf("relay already specified: %v", group)
			return 1
		}

		var ifs []*net.Interface

		for _, name := range parts[1:] {
			ifi, ok := ifm[name]
			if !ok {
				log.Printf("invalid interface: %v", name)
				return 1
			}

			ifs = append(ifs, ifi)
		}

		l, err := NewListener(ifs, group, id, ch)
		if err != nil {
			log.Printf("failed to create listener for %v on %v: %v", group, strings.Join(parts[1:], ","), err)
			return 1
		}

		listeners[group.Key()] = l

		s, err := NewSender(ifs, pool)
		if err != nil {
			log.Printf("failed to create sender for %v: %v", group, err)
			return 1
		}

		senders[group.Key()] = s

		log.Printf("relaying %v on %v", group, strings.Join(parts[1:], ","))
	}

	for _, l := range listeners {
		go l.Listen()
	}

	for msg := range ch {
		switch v := msg.(type) {
		case Packet:
			s := senders[v.Info.Dst.Key()]
			if s == nil {
				break
			}

			if verbose {
				log.Printf("RX: src=%v dst=%v if=%d dscp=%d ttl=%d len=%d", v.Info.Src, v.Info.Dst, v.Info.IfIndex, v.Info.DSCP, v.Info.TTL, len(v.Payload))
			}

			// Override DSCP to identify our own packets
			v.Info.DSCP = id

			errs := s.Send(v.Info, v.Payload)
			bytebufferpool.Put(v.Buffer)

			for _, err := range errs {
				log.Printf("TX error: %v", err)
			}

		case error:
			log.Printf("RX error: %v", v)
		}
	}

	return 0
}

func interfaces() (map[string]*net.Interface, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	m := make(map[string]*net.Interface)
	const want = net.FlagUp | net.FlagMulticast

	for i := range ifs {
		ifi := &ifs[i]

		if ifi.Flags & want != want {
			continue
		}

		if ifi.Flags & net.FlagLoopback != 0 {
			continue
		}

		m[ifi.Name] = ifi
	}

	return m, nil
}
