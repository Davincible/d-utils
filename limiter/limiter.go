package limiter

import (
	"sync"
	"time"
)

// RateLimit defines the maximum number of requests that can be made per time interval.
type RateLimit struct {
	Interval time.Duration
	MaxCount int
}

// RateLimiter is a simple rate limiter that keeps track of the number of requests per user per time interval.
type RateLimiter struct {
	mu              sync.Mutex
	users           map[string]*userData
	rateLimits      []RateLimit
	cleanupInterval time.Duration
}

// userData stores the request counts and last request times for each rate limit interval.
type userData struct {
	requestCounts map[time.Duration]int
	lastRequests  map[time.Duration]time.Time
}

// NewRateLimiter creates a new RateLimiter instance.
func NewRateLimiter(cleanupInterval time.Duration, rateLimits ...RateLimit) *RateLimiter {
	rl := &RateLimiter{
		users:           make(map[string]*userData),
		rateLimits:      rateLimits,
		cleanupInterval: cleanupInterval,
	}
	go rl.cleanupRoutine()
	return rl
}

// cleanupRoutine periodically removes expired user data.
func (rl *RateLimiter) cleanupRoutine() {
	for {
		time.Sleep(rl.cleanupInterval)
		rl.mu.Lock()
		now := time.Now()
		for userID, data := range rl.users {
			expired := true
			for _, limit := range rl.rateLimits {
				if now.Sub(data.lastRequests[limit.Interval]) < limit.Interval {
					expired = false
					break
				}
			}
			if expired {
				delete(rl.users, userID)
			}
		}
		rl.mu.Unlock()
	}
}

// AllowRequest checks if a request is allowed for the given uniqueID.
func (rl *RateLimiter) AllowRequest(uniqueID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	user, exists := rl.users[uniqueID]

	if !exists {
		user = &userData{
			requestCounts: make(map[time.Duration]int),
			lastRequests:  make(map[time.Duration]time.Time),
		}
		rl.users[uniqueID] = user
	}

	for _, limit := range rl.rateLimits {
		if _, ok := user.lastRequests[limit.Interval]; !ok {
			user.lastRequests[limit.Interval] = now
		}

		if now.Sub(user.lastRequests[limit.Interval]) >= limit.Interval {
			user.requestCounts[limit.Interval] = 0
			user.lastRequests[limit.Interval] = now
		}

		if user.requestCounts[limit.Interval] >= limit.MaxCount {
			return false
		}

		user.requestCounts[limit.Interval]++
	}

	return true
}

// NextActionTime returns the time when the next request will be allowed for the given uniqueID.
func (rl *RateLimiter) NextActionTime(userID string) time.Time {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	user, exists := rl.users[userID]
	if !exists {
		return time.Now()
	}

	var nextActionTime time.Time
	for _, limit := range rl.rateLimits {
		limitEndTime := user.lastRequests[limit.Interval].Add(limit.Interval)
		if user.requestCounts[limit.Interval] >= limit.MaxCount {
			if nextActionTime.IsZero() || limitEndTime.Before(nextActionTime) {
				nextActionTime = limitEndTime
			}
		}
	}

	if nextActionTime.IsZero() {
		return time.Now()
	}

	return nextActionTime
}
