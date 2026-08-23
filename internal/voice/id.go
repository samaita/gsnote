package voice

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

const counterFile = "_counter.txt"

// IDManager manages sequential voice IDs persisted to disk.
type IDManager struct {
	root string
	mu   sync.Mutex
}

// NewIDManager creates an IDManager for the given root directory.
func NewIDManager(root string) *IDManager {
	return &IDManager{root: root}
}

// Next returns the next sequential voice ID (zero-padded to 5 digits).
func (m *IDManager) Next() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	counter := m.readCounter()
	counter++
	if err := m.writeCounter(counter); err != nil {
		return "", err
	}
	return fmt.Sprintf("%05d", counter), nil
}

func (m *IDManager) readCounter() int {
	path := filepath.Join(m.root, counterFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(string(data))
	if err != nil {
		return 0
	}
	return n
}

func (m *IDManager) writeCounter(n int) error {
	if err := os.MkdirAll(m.root, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", m.root, err)
	}
	path := filepath.Join(m.root, counterFile)
	return os.WriteFile(path, []byte(fmt.Sprintf("%d", n)), 0644)
}
