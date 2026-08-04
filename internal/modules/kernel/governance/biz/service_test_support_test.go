package governance

import magicevent "github.com/muidea/magicCommon/event"

// NewService is intentionally test-only. Production Governance assembly must
// use NewEventService so owner reads and cross-component commands cannot
// bypass EventHub contracts.

var governanceTestNotificationHub = magicevent.NewHub(256)

func NewService(deps servicePorts) *Service {
	if deps.NotificationHub == nil {
		deps.NotificationHub = governanceTestNotificationHub
	}
	return newServiceWithPorts(deps)
}
