package ledsign

// https://www.reddit.com/r/golang/comments/jeqmtt/wake_up_at_time/

import (
	"time"

	"golang.org/x/sys/unix"
)

func sleepUntil(t time.Time) error {
	req, err := unix.TimeToTimespec(t)
	if err != nil {
			return err
	}
	var rem *unix.Timespec
	for {
			err := unix.ClockNanosleep(unix.CLOCK_REALTIME, unix.TIMER_ABSTIME, &req, rem)
			if err != nil {
					if err == unix.EINTR {
							// BUG: req is already absolute?
							req = *rem
							continue
					}
					return err
			}
			return nil
	}
}
