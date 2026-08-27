package adapter

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Registry struct {
	provider Provider

	mu         sync.RWMutex
	adapters   []Adapter
	selectedID string
	override   string
}

func NewRegistry(provider Provider) *Registry {
	return &Registry{provider: provider}
}

func (r *Registry) Refresh(ctx context.Context) ([]Adapter, error) {
	values, err := r.provider.List(ctx)
	if err != nil {
		return nil, err
	}
	for index := range values {
		values[index].Score, values[index].USBCandidate = ScoreUSBAdapter(values[index])
	}
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].USBCandidate != values[j].USBCandidate {
			return values[i].USBCandidate
		}
		if values[i].Score != values[j].Score {
			return values[i].Score > values[j].Score
		}
		return values[i].InterfaceIndex < values[j].InterfaceIndex
	})

	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters = cloneAdapters(values)
	r.selectedID = ""
	if r.override != "" {
		if selected, ok := findAdapter(values, r.override); ok && selected.IsUp() {
			r.selectedID = selected.ID
		}
	} else {
		for _, value := range values {
			if value.IsUp() && value.USBCandidate {
				r.selectedID = value.ID
				break
			}
		}
	}
	return cloneAdapters(values), nil
}

func (r *Registry) Run(ctx context.Context, interval time.Duration, onSelectionChanged func(*Adapter)) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	previous := ""
	refresh := func() {
		_, _ = r.Refresh(ctx)
		selected, ok := r.Selected()
		current := ""
		if ok {
			current = fmt.Sprintf("%s:%d", selected.ID, selected.InterfaceIndex)
		}
		if current != previous && onSelectionChanged != nil {
			if ok {
				copy := selected
				onSelectionChanged(&copy)
			} else {
				onSelectionChanged(nil)
			}
		}
		previous = current
	}
	refresh()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refresh()
		}
	}
}

func (r *Registry) Select(selector string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := findAdapter(r.adapters, selector)
	if !ok {
		return fmt.Errorf("network adapter %q was not found", selector)
	}
	r.override = value.ID
	if value.IsUp() {
		r.selectedID = value.ID
	} else {
		r.selectedID = ""
	}
	return nil
}

func (r *Registry) UseAutomaticSelection() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.override = ""
	r.selectedID = ""
	for _, value := range r.adapters {
		if value.IsUp() && value.USBCandidate {
			r.selectedID = value.ID
			break
		}
	}
}

func (r *Registry) Selected() (Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.selectedID == "" {
		return Adapter{}, false
	}
	value, ok := findAdapter(r.adapters, r.selectedID)
	return value, ok
}

func (r *Registry) Snapshot() []Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneAdapters(r.adapters)
}

func findAdapter(values []Adapter, selector string) (Adapter, bool) {
	selector = strings.TrimSpace(selector)
	for _, value := range values {
		if strings.EqualFold(value.ID, selector) ||
			strings.EqualFold(value.Name, selector) ||
			strconv.Itoa(value.InterfaceIndex) == selector {
			return value, true
		}
	}
	return Adapter{}, false
}

func cloneAdapters(values []Adapter) []Adapter {
	result := make([]Adapter, len(values))
	for index, value := range values {
		value.IPv4 = append([]string(nil), value.IPv4...)
		value.IPv6 = append([]string(nil), value.IPv6...)
		value.Gateways = append([]string(nil), value.Gateways...)
		value.DNSServers = append([]string(nil), value.DNSServers...)
		result[index] = value
	}
	return result
}
