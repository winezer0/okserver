package server

import "net"

func internalIP() (ips []net.IP) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return
	}

	for _, inf := range interfaces {
		if inf.Flags&net.FlagUp != net.FlagUp || inf.Flags&net.FlagLoopback == net.FlagLoopback {
			continue
		}

		addr, err := inf.Addrs()
		if err != nil {
			continue
		}

		for _, a := range addr {
			if ipNet, ok := a.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
				ips = append(ips, ipNet.IP)
			}
		}
	}
	return
}
