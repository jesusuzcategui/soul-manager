package ui

import (
	"strconv"
	"strings"
	"sync"

	"github.com/jesusuzcategui/soul-app/internal/model"
)

// State provides thread-safe management of discovered processes and search query state.
type State struct {
	mu       sync.RWMutex
	all      []model.PortProcess
	filtered []model.PortProcess
	query    string
}

// NewState creates and initializes a new thread-safe State instance.
func NewState() *State {
	return &State{
		all:      make([]model.PortProcess, 0),
		filtered: make([]model.PortProcess, 0),
		query:    "",
	}
}

// SetProcesses updates the master list of processes and recalculates the filtered subset.
func (s *State) SetProcesses(procs []model.PortProcess) []model.PortProcess {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Defensive copy to prevent external mutation
	copied := make([]model.PortProcess, len(procs))
	copy(copied, procs)
	s.all = copied

	s.filtered = filterProcesses(s.all, s.query)
	result := make([]model.PortProcess, len(s.filtered))
	copy(result, s.filtered)
	return result
}

// SetQuery updates the search filter query and recalculates the filtered subset.
func (s *State) SetQuery(query string) []model.PortProcess {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.query = query
	s.filtered = filterProcesses(s.all, s.query)
	result := make([]model.PortProcess, len(s.filtered))
	copy(result, s.filtered)
	return result
}

// GetFiltered returns a thread-safe copy of the currently filtered processes.
func (s *State) GetFiltered() []model.PortProcess {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]model.PortProcess, len(s.filtered))
	copy(result, s.filtered)
	return result
}

// GetAll returns a thread-safe copy of all tracked processes.
func (s *State) GetAll() []model.PortProcess {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]model.PortProcess, len(s.all))
	copy(result, s.all)
	return result
}

// GetQuery returns the current search query string.
func (s *State) GetQuery() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.query
}

// Count returns the number of filtered processes and total processes.
func (s *State) Count() (filtered int, total int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.filtered), len(s.all)
}

// MatchesFilter checks whether a PortProcess matches the given query.
// It performs case-insensitive matching across Port, PID, Process Name,
// Container Type, Command-line, and Protocol.
func MatchesFilter(p model.PortProcess, query string) bool {
	cleanQuery := strings.ToLower(strings.TrimSpace(query))
	if cleanQuery == "" {
		return true
	}

	// Match Port
	portStr := strconv.Itoa(p.Port)
	if strings.Contains(portStr, cleanQuery) {
		return true
	}

	// Match PID
	if p.PID > 0 {
		pidStr := strconv.Itoa(int(p.PID))
		if strings.Contains(pidStr, cleanQuery) {
			return true
		}
	}

	// Match Process Name
	if strings.Contains(strings.ToLower(p.Name), cleanQuery) {
		return true
	}

	// Match resolved container name
	if strings.Contains(strings.ToLower(p.ContainerName), cleanQuery) {
		return true
	}

	// Match Container Type or Container badge terms
	if strings.Contains(strings.ToLower(p.ContainerType), cleanQuery) {
		return true
	}
	if p.IsContainer && cleanQuery == "container" {
		return true
	}

	// Match Command-line
	if strings.Contains(strings.ToLower(p.Cmdline), cleanQuery) {
		return true
	}

	// Match Protocol (e.g. "tcp", "tcp6")
	if strings.Contains(strings.ToLower(p.Protocol), cleanQuery) {
		return true
	}

	return false
}

// filterProcesses returns a new slice containing only items matching query.
func filterProcesses(procs []model.PortProcess, query string) []model.PortProcess {
	cleanQuery := strings.ToLower(strings.TrimSpace(query))
	if cleanQuery == "" {
		res := make([]model.PortProcess, len(procs))
		copy(res, procs)
		return res
	}

	matched := make([]model.PortProcess, 0, len(procs))
	for _, p := range procs {
		if MatchesFilter(p, cleanQuery) {
			matched = append(matched, p)
		}
	}
	return matched
}
