package core

import "time"

// HitTakenStandIn replaces the damage a healer sim never deals its raid, for effects that wait on a hit taken:
// once started it calls onHit every 60 / perMinute seconds, the first one a whole period after Start. A rate of 0
// never fires.
type HitTakenStandIn struct {
	period time.Duration
	onHit  func(sim *Simulation)
	action *PendingAction
}

func NewHitTakenStandIn(perMinute float64, onHit func(sim *Simulation)) *HitTakenStandIn {
	standIn := &HitTakenStandIn{onHit: onHit}
	if perMinute > 0 {
		standIn.period = time.Duration(float64(time.Minute) / perMinute)
	}
	return standIn
}

// Start restarts the clock if the stand-in is running.
func (standIn *HitTakenStandIn) Start(sim *Simulation) {
	standIn.Stop(sim)
	if standIn.period <= 0 {
		return
	}
	standIn.action = StartPeriodicAction(sim, PeriodicActionOptions{
		Period:   standIn.period,
		OnAction: standIn.onHit,
	})
}

func (standIn *HitTakenStandIn) Stop(sim *Simulation) {
	if standIn.action != nil {
		standIn.action.Cancel(sim)
		standIn.action = nil
	}
}
