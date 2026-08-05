package eventstream

import "github.com/muidea/workorch/internal/pkg/workorcheventrecordcontract"

// EventRecord is the Eventstream owner's persisted record type. Its immutable
// field contract is shared without exposing repository or owner behavior.
type EventRecord workorcheventrecordcontract.Record
