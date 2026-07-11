package governance

import magicevent "github.com/muidea/magicCommon/event"

// Dependencies and NewService are intentionally test-only. Production
// Governance assembly must use NewEventService so owner reads and
// cross-component commands cannot bypass EventHub contracts.
type Dependencies = servicePorts

var governanceTestNotificationHub = magicevent.NewHub(256)

func NewService(deps Dependencies) *Service {
	if deps.NotificationHub == nil {
		deps.NotificationHub = governanceTestNotificationHub
	}
	return newServiceWithPorts(deps)
}
