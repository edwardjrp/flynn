package health

import (
	"time"

	"github.com/flynn/flynn/pkg/stream"
)

type Monitor struct {
	// StartInterval is the check interval to use when waiting for the service
	// to transition from created -> up. It defaults to 100ms.
	StartInterval time.Duration

	// Interval is the check interval to use when the service is up or down. It
	// defaults to two seconds.
	Interval time.Duration

	// Threshold is the number of consecutive checks of the same status before
	// a service will transition up -> down or down -> up. It defaults to 2.
	Threshold int
}

type MonitorStatus int

const (
	MonitorStatusUnknown MonitorStatus = iota
	MonitorStatusCreated
	MonitorStatusUp
	MonitorStatusDown
)

type MonitorEvent struct {
	Status MonitorStatus
	// If Status is MonitorStatusDown, Err is the last failure
	Err error
	// Check is included to identify the monitor.
	Check Check
}

const (
	defaultStartInterval = 100 * time.Millisecond
	defaultInterval      = 2 * time.Second
	defaultThreshold     = 2
)

// Run monitors a service using Check and sends up/down transitions to ch
func (m Monitor) Run(check Check, ch chan MonitorEvent) stream.Stream {
	if m.StartInterval == 0 {
		m.StartInterval = defaultStartInterval
	}
	if m.Interval == 0 {
		m.Interval = defaultInterval
	}
	if m.Threshold == 0 {
		m.Threshold = defaultThreshold
	}

	stream := stream.New()
	go func() {
		t := time.NewTicker(m.StartInterval)
		defer close(ch)

		status := MonitorStatusCreated
		var upCount, downCount int
		up := func() {
			downCount = 0
			upCount++
			if status == MonitorStatusCreated || status == MonitorStatusDown && upCount >= m.Threshold {
				if status == MonitorStatusCreated {
					t.Stop()
					t = time.NewTicker(m.Interval)
				}
				status = MonitorStatusUp
				ch <- MonitorEvent{
					Status: status,
					Check:  check,
				}
			}
		}
		down := func(err error) {
			upCount = 0
			downCount++
			if status == MonitorStatusUp && downCount >= m.Threshold {
				status = MonitorStatusDown
				ch <- MonitorEvent{
					Status: status,
					Err:    err,
					Check:  check,
				}
			}
		}
		check := func() {
			if err := check.Check(); err != nil {
				down(err)
			} else {
				up()
			}
		}

		check()
	outer:
		for {
			select {
			case <-t.C:
				check()
			case <-stream.StopCh:
				break outer
			}
		}
		t.Stop()
	}()

	return stream
}
