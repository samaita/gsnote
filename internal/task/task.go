package task

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	taskUnchecked   = "- [ ] "
	taskChecked     = "- [x] "
	taskFileExt     = ".md"
	taskHeaderFmt   = "# Tasks: %s\n\n"
	taskTextOffset  = len(taskUnchecked) // same length as taskChecked
	taskStatusStart = 2
	taskStatusEnd   = 5
)

type Manager struct {
	tasksRoot string
}

func New(tasksRoot string) *Manager {
	return &Manager{tasksRoot: tasksRoot}
}

func (m *Manager) filePath(date string) string {
	return filepath.Join(m.tasksRoot, date+taskFileExt)
}

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines, nil
}

func writeLines(path string, lines []string) error {
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

func isTaskLine(line string) bool {
	return strings.HasPrefix(line, taskUnchecked) || strings.HasPrefix(line, taskChecked)
}

func taskIndices(lines []string) []int {
	var idx []int
	for i, line := range lines {
		if isTaskLine(line) {
			idx = append(idx, i)
		}
	}
	return idx
}

func (m *Manager) Add(date, text string) error {
	path := m.filePath(date)

	_, statErr := os.Stat(path)
	isNew := os.IsNotExist(statErr)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if isNew {
		fmt.Fprintf(f, taskHeaderFmt, date)
	}
	fmt.Fprintf(f, "%s%s\n", taskUnchecked, text)
	return nil
}

func (m *Manager) View(date string) (string, error) {
	path := m.filePath(date)
	lines, err := readLines(path)
	if os.IsNotExist(err) {
		return fmt.Sprintf("No tasks for %s.", date), nil
	}
	if err != nil {
		return "", err
	}

	indices := taskIndices(lines)
	if len(indices) == 0 {
		return fmt.Sprintf("No tasks for %s.", date), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Tasks %s:\n", date))
	for i, idx := range indices {
		line := lines[idx]
		status := line[taskStatusStart:taskStatusEnd]
		text := line[taskTextOffset:]
		sb.WriteString(fmt.Sprintf("%d. %s %s\n", i+1, status, text))
	}

	return strings.TrimRight(sb.String(), "\n"), nil
}

func (m *Manager) Done(date string, n int) error {
	path := m.filePath(date)
	lines, err := readLines(path)
	if err != nil {
		return err
	}

	indices := taskIndices(lines)
	if n < 1 || n > len(indices) {
		return fmt.Errorf("task %d not found", n)
	}

	idx := indices[n-1]
	line := lines[idx]
	if strings.HasPrefix(line, taskUnchecked) {
		lines[idx] = taskChecked + line[taskTextOffset:]
	} else {
		lines[idx] = taskUnchecked + line[taskTextOffset:]
	}

	return writeLines(path, lines)
}

func (m *Manager) Edit(date string, n int, newText string) error {
	path := m.filePath(date)
	lines, err := readLines(path)
	if err != nil {
		return err
	}

	indices := taskIndices(lines)
	if n < 1 || n > len(indices) {
		return fmt.Errorf("task %d not found", n)
	}

	idx := indices[n-1]
	status := lines[idx][taskStatusStart:taskStatusEnd]
	lines[idx] = fmt.Sprintf("- %s %s", status, newText)

	return writeLines(path, lines)
}

func (m *Manager) Delete(date string, n int) error {
	path := m.filePath(date)
	lines, err := readLines(path)
	if err != nil {
		return err
	}

	indices := taskIndices(lines)
	if n < 1 || n > len(indices) {
		return fmt.Errorf("task %d not found", n)
	}

	idx := indices[n-1]
	lines = append(lines[:idx], lines[idx+1:]...)

	return writeLines(path, lines)
}
