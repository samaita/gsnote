package jobs

import "time"

func RetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		return 0
	}
	delay := RetryBase
	for attempt > 1 && delay < RetryMax {
		if delay >= RetryMax/2 {
			return RetryMax
		}
		delay *= 2
		attempt--
	}
	return delay
}
