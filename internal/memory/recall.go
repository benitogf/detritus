package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/benitogf/detritus/internal/core"
)

// The recall-miss counter is the falsifiable trigger for whether a dense
// (vector) retrieval arm is ever worth building — NOT a dormant code seam.
// Every skill_search logs its outcome (on-session, zero metered cost); a miss
// is an empty top-k. Only if a sustained miss fraction shows up at scale does a
// dense arm earn a head-to-head bake-off. Until then there is no vector index,
// no embedding model, no seam.

func recallLogPath() string { return filepath.Join(MemoryDir(), "recall.log") }

// LogSearch appends one retrieval outcome to the recall log: timestamp, result
// count, and the (newline-stripped) query. A zero count is a recall miss.
func LogSearch(query string, nResults int) error {
	if err := ensureStore(); err != nil {
		return err
	}
	line := fmt.Sprintf("%s\t%d\t%s\n", nowRFC3339(), nResults, strings.ReplaceAll(query, "\n", " "))
	f, err := os.OpenFile(recallLogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}

// Recall summarizes the recall log.
type Recall struct {
	Total  int
	Misses int
}

// MissFraction is misses/total (0 when no searches logged).
func (r Recall) MissFraction() float64 {
	if r.Total == 0 {
		return 0
	}
	return float64(r.Misses) / float64(r.Total)
}

// RecallStats reads the recall log and counts total searches and misses
// (empty top-k). This is the measurement that decides the dense-arm question.
func RecallStats() (Recall, error) {
	data, err := os.ReadFile(recallLogPath())
	if err != nil {
		if os.IsNotExist(err) {
			return Recall{}, nil
		}
		return Recall{}, err
	}
	var r Recall
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		r.Total++
		if n, err := strconv.Atoi(parts[1]); err == nil && n == 0 {
			r.Misses++
		}
	}
	return r, nil
}

// SearchAndLog runs a search and records its outcome to the recall counter. The
// skill_search tool calls this so every retrieval is logged.
func SearchAndLog(query string, topN int) ([]core.Result, error) {
	results, err := Search(query, topN)
	if err != nil {
		return nil, err
	}
	_ = LogSearch(query, len(results)) // best-effort instrumentation
	return results, nil
}
