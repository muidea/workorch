package tui

import (
	"strings"
	"testing"
	"time"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func useLocalTimezone(t *testing.T, zone string) {
	t.Helper()
	location, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	original := time.Local
	time.Local = location
	t.Cleanup(func() { time.Local = original })
}

func TestLocalDateTimeUsesCurrentTimezone(t *testing.T) {
	for _, tc := range []struct{ zone, input, expected, clock string }{
		{"Asia/Shanghai", "2026-11-01T18:30:45.123456789Z", "2026-11-02 02:30:45 +08:00", "02:30"},
		{"Asia/Shanghai", "2026-11-02T08:00:00+02:00", "2026-11-02 14:00:00 +08:00", "14:00"},
		{"Asia/Kathmandu", "2026-11-02T00:00:00Z", "2026-11-02 05:45:00 +05:45", "05:45"},
		{"America/St_Johns", "2026-11-02T00:00:00Z", "2026-11-01 20:30:00 -03:30", "20:30"},
		{"America/New_York", "2026-07-01T02:15:00Z", "2026-06-30 22:15:00 -04:00", "22:15"},
		{"America/New_York", "2026-11-02T02:15:00Z", "2026-11-01 21:15:00 -05:00", "21:15"},
		{"UTC", "2026-11-02T00:00:00Z", "2026-11-02 00:00:00 +00:00", "00:00"},
	} {
		t.Run(tc.zone+tc.input, func(t *testing.T) {
			useLocalTimezone(t, tc.zone)
			if got := localDateTime(tc.input); got != tc.expected {
				t.Fatalf("date = %q, want %q", got, tc.expected)
			}
			if got := activityTimeLabel(tc.input); got != tc.clock {
				t.Fatalf("clock = %q, want %q", got, tc.clock)
			}
		})
	}
}

func TestLocalTimePreservesUnparseableText(t *testing.T) {
	for _, input := range []string{"", "2026-11-02", "2026-11-02T09:00:00", "10:34:56", "  unknown  ", "2026-99-02T09:00:00Z"} {
		if localDateTime(input) != input || activityTimeLabel(input) != input {
			t.Fatalf("changed unparseable timestamp %q", input)
		}
	}
}

func TestTimeViewsUseLocalTimeWithoutChangingProjection(t *testing.T) {
	useLocalTimezone(t, "Asia/Shanghai")
	const original = "2026-11-01T18:30:45Z"
	feed := projectioncontract.WorkspaceActivityFeed{Items: []projectioncontract.WorkspaceActivityItem{{Summary: "work accepted", OccurredAt: original}}}
	if got := stripANSI(renderRecentActivityCompact(feed, 100, 5)); !strings.Contains(got, "02:30 work accepted") || strings.Contains(got, "18:30") {
		t.Fatal(got)
	}
	m := AppModel{
		runHeartbeat:      projectioncontract.RunHeartbeat{ActivityState: "active", LatestActivityAt: original, LatestEffectiveProgressAt: original},
		convergenceStatus: &projectioncontract.ConvergenceStatus{LastProgressAt: original},
	}
	for _, got := range []string{renderInspectActivityFeed(feed, 5), m.renderInspectStatus(), convergenceHeartbeatText(m.convergenceStatus)} {
		if !strings.Contains(got, "2026-11-02 02:30:45 +08:00") || strings.Contains(got, original) {
			t.Fatal(got)
		}
	}
	if feed.Items[0].OccurredAt != original || m.runHeartbeat.LatestActivityAt != original || m.convergenceStatus.LastProgressAt != original {
		t.Fatal("rendering modified the owner projection")
	}
}
