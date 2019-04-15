package conf

var (
	// HostIPs contain Docker host ip address for monitor
	HostIPs = []string{"localhost"}
)

func IsKnownHost(host string) bool {
	for _, v := range HostIPs {
		if v == host {
			return true
		}
	}

	return false
}
