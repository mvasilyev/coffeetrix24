package scheduler

import (
	"context"
	"log"
	"strings"
	"time"

	"coffeetrix24/internal/db"
)

type Scheduler struct {
	Store *db.Store
	OnDailyInvite func()
	OnCloseSessions func(ids []int64)
	// Config
	CloseInterval time.Duration
	DisableDaily bool
}

func New(store *db.Store) *Scheduler {
	return &Scheduler{Store: store, CloseInterval: 30 * time.Second}
}

// Start runs scheduling loop for daily invite and session closing.
func (s *Scheduler) Start(ctx context.Context) {
	if !s.DisableDaily { go s.loopDaily(ctx) }
	go s.loopCloser(ctx)
}

func parseDaily(t string) (int, int) {
	parts := strings.Split(t, ":")
	if len(parts) != 2 { return 9, 0 }
	h, _ := time.Parse("15", parts[0])
	m, _ := time.Parse("04", parts[1])
	return h.Hour(), m.Minute()
}

func (s *Scheduler) loopDaily(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		daily, err := s.Store.GetDailyTime()
		if err != nil { time.Sleep(time.Minute); continue }
		hh, mm := parseDaily(daily)
		now := time.Now().UTC()
		next := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, time.UTC)
		if !next.After(now) { next = next.Add(24 * time.Hour) }
		d := time.Until(next)
		select {
		case <-ctx.Done():
			return
		case <-time.After(d):
			if s.OnDailyInvite != nil { s.OnDailyInvite() }
		}
	}
}

func (s *Scheduler) loopCloser(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
	case <-time.After(s.CloseInterval):
			now := time.Now().UTC()
			ids, err := s.Store.GetOpenSessionsToClose(now)
			if err != nil { log.Println("closer error:", err); continue }
			if len(ids) > 0 && s.OnCloseSessions != nil {
				s.OnCloseSessions(ids)
			}
		}
	}
}
