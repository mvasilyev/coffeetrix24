package scheduler

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"coffeetrix24/internal/db"
)

type Scheduler struct {
	Store           *db.Store
	OnDailyInvite   func()
	OnCloseSessions func(ids []int64)
	// Config
	CloseInterval time.Duration
	DisableDaily  bool
}

func New(store *db.Store) *Scheduler {
	return &Scheduler{Store: store, CloseInterval: 30 * time.Second}
}

// Start runs scheduling loop for daily invite and session closing.
func (s *Scheduler) Start(ctx context.Context) {
	if !s.DisableDaily {
		go s.loopDaily(ctx)
	}
	go s.loopCloser(ctx)
}

func parseDaily(t string) (int, int) {
	parts := strings.Split(t, ":")
	if len(parts) != 2 {
		return 9, 0
	}
	hh, err1 := strconv.Atoi(parts[0])
	mm, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 9, 0
	}
	if hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 9, 0
	}
	return hh, mm
}

func (s *Scheduler) loopDaily(ctx context.Context) {
	// Timer that re-reads settings every minute and reschedules if time changed.
	getNext := func(hh, mm int, from time.Time) time.Time {
		n := time.Date(from.Year(), from.Month(), from.Day(), hh, mm, 0, 0, time.UTC)
		if !n.After(from) {
			n = n.Add(24 * time.Hour)
		}
		return n
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	// initial schedule
	daily, err := s.Store.GetDailyTime()
	if err != nil {
		daily = "09:00"
	}
	hh, mm := parseDaily(daily)
	now := time.Now().UTC()
	next := getNext(hh, mm, now)
	timer := time.NewTimer(time.Until(next))
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if s.OnDailyInvite != nil {
				s.OnDailyInvite()
			}
			// after firing, compute next based on current setting
			now = time.Now().UTC()
			daily, err = s.Store.GetDailyTime()
			if err != nil {
				daily = "09:00"
			}
			hh, mm = parseDaily(daily)
			next = getNext(hh, mm, now)
			timer = time.NewTimer(time.Until(next))
		case <-ticker.C:
			// check if time changed and reschedule
			daily2, err2 := s.Store.GetDailyTime()
			if err2 != nil {
				continue
			}
			h2, m2 := parseDaily(daily2)
			newNext := getNext(h2, m2, time.Now().UTC())
			// if scheduling changed, reset timer
			if !newNext.Equal(next) {
				next = newNext
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer = time.NewTimer(time.Until(next))
			}
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
			if err != nil {
				log.Println("closer error:", err)
				continue
			}
			if len(ids) > 0 && s.OnCloseSessions != nil {
				s.OnCloseSessions(ids)
			}
		}
	}
}
