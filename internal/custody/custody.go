// Package custody interprets custody_* records: who held which item, when, and
// through which documented handover. The ledger itself lives in custody.jsonl
// and is written through internal/chain.
package custody

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/subsll/stratum/internal/chain"
	"github.com/subsll/stratum/internal/model"
)

// Event is one parsed custody record.
type Event struct {
	Seq  int               `json:"seq"`
	Time string            `json:"time"`
	Type string            `json:"type"`
	Hash string            `json:"hash"`
	Data model.CustodyData `json:"data"`
}

// Ledger is the interpreted state of a custody log.
type Ledger struct {
	Events       []Event
	HolderByItem map[string]string
	CaseHolder   string
	Warnings     []string
}

// Analyze interprets records against the acquired items. defaultHolder is used
// for items that predate any collection event (normally the case examiner).
func Analyze(records []chain.Record, items []model.Item, defaultHolder string) *Ledger {
	l := &Ledger{
		HolderByItem: make(map[string]string, len(items)),
		CaseHolder:   defaultHolder,
	}
	for _, it := range items {
		l.HolderByItem[it.ID] = defaultHolder
	}
	itemByID := make(map[string]model.Item, len(items))
	for _, it := range items {
		itemByID[it.ID] = it
	}

	for _, rec := range records {
		if !strings.HasPrefix(rec.Type, "custody_") {
			continue
		}
		var data model.CustodyData
		if err := json.Unmarshal(rec.Data, &data); err != nil {
			l.Warnings = append(l.Warnings, fmt.Sprintf("custody record %d cannot be read: %v", rec.Seq, err))
			continue
		}
		action := data.Action
		if action == "" {
			action = strings.TrimPrefix(rec.Type, "custody_")
		}
		ev := Event{Seq: rec.Seq, Time: rec.Time, Type: rec.Type, Hash: rec.Hash, Data: data}
		l.Events = append(l.Events, ev)

		scope, scopeWarnings := scopeItems(data, itemByID, rec.Time)
		l.Warnings = append(l.Warnings, scopeWarnings...)
		to := strings.TrimSpace(data.To.Display())
		if to == "" {
			to = strings.TrimSpace(data.From)
		}
		switch action {
		case "opened", "collected", "transferred", "returned":
			if to == "" {
				l.Warnings = append(l.Warnings, fmt.Sprintf("custody record %d has no recipient", rec.Seq))
				continue
			}
			for _, id := range scope {
				l.HolderByItem[id] = to
			}
			if action == "opened" || len(scope) == len(items) {
				l.CaseHolder = to
			}
		}
	}
	return l
}

// scopeItems resolves "*" (all items acquired before the event) or an explicit
// item list. Unknown explicit IDs produce a warning.
func scopeItems(data model.CustodyData, items map[string]model.Item, eventTime string) ([]string, []string) {
	wildcard := len(data.Items) == 0
	for _, id := range data.Items {
		if id == "*" {
			wildcard = true
			break
		}
	}
	if wildcard {
		t, err := time.Parse(time.RFC3339Nano, eventTime)
		out := make([]string, 0, len(items))
		for id, it := range items {
			if err != nil {
				out = append(out, id)
				continue
			}
			at, aerr := time.Parse(time.RFC3339Nano, it.AcquiredAt)
			if aerr != nil || !at.After(t) {
				out = append(out, id)
			}
		}
		sort.Strings(out)
		return out, nil
	}
	var out []string
	var warnings []string
	seen := make(map[string]bool)
	var eventAt time.Time
	var eventTimeOK bool
	if t, err := time.Parse(time.RFC3339Nano, eventTime); err == nil {
		eventAt, eventTimeOK = t, true
	}
	for _, id := range data.Items {
		it, ok := items[id]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("custody record references unknown item %s", id))
			continue
		}
		if eventTimeOK {
			if at, err := time.Parse(time.RFC3339Nano, it.AcquiredAt); err == nil && at.After(eventAt) {
				warnings = append(warnings, fmt.Sprintf("custody record covers %s before its acquisition time", id))
			}
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out, warnings
}

// Applies reports whether an event covers an item, including wildcard events
// that occurred after the item was acquired.
func (l *Ledger) Applies(ev Event, item model.Item) bool {
	for _, id := range ev.Data.Items {
		if id == item.ID {
			return true
		}
	}
	for _, id := range ev.Data.Items {
		if id == "*" {
			t, err := time.Parse(time.RFC3339Nano, ev.Time)
			if err != nil {
				return true
			}
			at, aerr := time.Parse(time.RFC3339Nano, item.AcquiredAt)
			return aerr != nil || !at.After(t)
		}
	}
	return false
}

// History returns custody events that apply to one item.
func (l *Ledger) History(item model.Item) []Event {
	var out []Event
	for _, ev := range l.Events {
		if l.Applies(ev, item) {
			out = append(out, ev)
		}
	}
	return out
}

// HolderCounts groups items by their current holder.
type HolderCount struct {
	Holder string `json:"holder"`
	Count  int    `json:"count"`
}

// HolderCounts returns current holders, largest group first.
func (l *Ledger) HolderCounts() []HolderCount {
	counts := map[string]int{}
	for _, holder := range l.HolderByItem {
		counts[holder]++
	}
	out := make([]HolderCount, 0, len(counts))
	for holder, count := range counts {
		out = append(out, HolderCount{Holder: holder, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Holder < out[j].Holder
		}
		return out[i].Count > out[j].Count
	})
	return out
}
