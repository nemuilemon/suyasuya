package service

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"time"

	"suyasuya/internal/domain"
	"suyasuya/internal/platform"
)

func measure(in *domain.PVTInput) (domain.Metrics, error) {
	m := domain.Metrics{}
	duration := in.Finished.Sub(in.Started).Seconds() * 1000
	if duration < 0 || duration > 65000 || in.Started.After(time.Now().Add(time.Minute)) || len(in.Trials) > 2000 {
		return m, domain.Invalid("測定時間が不正です")
	}
	if in.Completed && duration < 60000 {
		return m, domain.Invalid("1分未満の測定は中断として保存してください")
	}
	values := []float64{}
	last := -1.0
	for i := range in.Trials {
		t := &in.Trials[i]
		if math.IsNaN(t.Response) || math.IsInf(t.Response, 0) || t.Response < 0 || t.Response > duration+100 || t.Response <= last {
			return m, domain.Invalid("反応の順序が不正です")
		}
		last = t.Response
		if t.Stimulus == nil {
			t.False = true
			t.RT = nil
		} else {
			if *t.Stimulus < 0 || *t.Stimulus > t.Response || math.IsNaN(*t.Stimulus) || math.IsInf(*t.Stimulus, 0) {
				return m, domain.Invalid("刺激の時刻が不正です")
			}
			rt := t.Response - *t.Stimulus
			t.RT = &rt
			t.False = rt < 100
			if !t.False {
				values = append(values, rt)
				if rt >= 355 {
					m.Lapses++
				}
			}
		}
		if t.False {
			m.False++
		}
	}
	sort.Float64s(values)
	n := len(values)
	if n > 0 {
		median := values[n/2]
		if n%2 == 0 {
			median = (values[n/2-1] + median) / 2
		}
		m.Median = &median
		count := int(math.Ceil(float64(n) * 0.1))
		sum := 0.0
		for _, v := range values[n-count:] {
			sum += v
		}
		sum /= float64(count)
		m.Slow = &sum
	}
	return m, nil
}
func (a *Control) PVTList(ctx context.Context, u domain.User) (any, error) {
	out, e := a.repo.PVTList(ctx, u.ID)
	if e != nil {
		return nil, e
	}
	return out, nil
}

func (a *Control) PVTSave(ctx context.Context, u domain.User, in domain.PVTInput) (any, error) {
	if !platform.ValidID(in.ID) {
		return nil, domain.Invalid("測定IDが不正です")
	}
	if _, e := time.LoadLocation(in.Timezone); e != nil {
		return nil, domain.Invalid("タイムゾーンを確認してください")
	}
	if len(in.Device) > 256 || (in.Input != "keyboard" && in.Input != "pointer" && in.Input != "mixed") {
		return nil, domain.Invalid("測定端末が不正です")
	}
	m, e := measure(&in)
	if e != nil {
		return nil, e
	}
	b, e := json.Marshal(in.Trials)
	if e != nil {
		return nil, e
	}
	_, e = a.repo.InsertPVT(ctx, in.ID, u.ID, in.Started, in.Finished, in.Timezone, in.Device, in.Input, string(b), in.Completed, m.Median, m.Lapses, m.Slow, m.False)
	if e != nil {
		return nil, e
	}
	return m, nil
}
