package scheduler

import (
	"fmt"
	"os"
	"time"
)

// Config carries the scheduler's tunables.
//
// Fields:
//   - Location - the time zone that decides what "today" is: a subscription
//     ending on the 30th has expired once the 31st starts there.
//   - Interval - how often a run happens; the first one runs at startup.
type Config struct {
	Location *time.Location
	Interval time.Duration
}

// LoadConfig reads SCHEDULER_* env vars into a Config.
//
// Recognised env vars:
//   - SCHEDULER_TZ       (IANA zone name, default Europe/Moscow)
//   - SCHEDULER_INTERVAL (time.Duration, default 1h, at least 1m)
//
// Returns an error for an unknown zone or a malformed or too short interval.
func LoadConfig() (Config, error) {
	tz := os.Getenv("SCHEDULER_TZ")
	if tz == "" {
		tz = "Europe/Moscow"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return Config{}, fmt.Errorf("invalid SCHEDULER_TZ %q: %w", tz, err)
	}
	c := Config{Location: loc, Interval: time.Hour}
	if v := os.Getenv("SCHEDULER_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid SCHEDULER_INTERVAL %q: %w", v, err)
		}
		if d < time.Minute {
			return Config{}, fmt.Errorf("SCHEDULER_INTERVAL must be at least 1m, got %s", d)
		}
		c.Interval = d
	}
	return c, nil
}
