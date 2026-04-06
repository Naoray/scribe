package workflow

import (
	"testing"
	"time"
)

func TestTimeAgo(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name string
		t    time.Time
		want string
	}{
		{"zero value", time.Time{}, "never synced"},
		{"just now", now.Add(-30 * time.Second), "just now"},
		{"minutes ago", now.Add(-25 * time.Minute), "25 minutes ago"},
		{"1 minute ago", now.Add(-90 * time.Second), "1 minute ago"},
		{"hours ago", now.Add(-3 * time.Hour), "3 hours ago"},
		{"1 hour ago", now.Add(-90 * time.Minute), "1 hour ago"},
		{"days ago", now.Add(-5 * 24 * time.Hour), "5 days ago"},
		{"1 day ago", now.Add(-36 * time.Hour), "1 day ago"},
		{"old date", now.Add(-45 * 24 * time.Hour), now.Add(-45 * 24 * time.Hour).Format("2006-01-02")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := TimeAgo(c.t)
			if got != c.want {
				t.Errorf("timeAgo(%v) = %q, want %q", c.t, got, c.want)
			}
		})
	}
}
