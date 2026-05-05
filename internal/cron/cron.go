package cron

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const FileName = "cron.txt"

var (
	ErrInvalidSpec      = errors.New("invalid cron spec")
	ErrUnsupportedCmd   = errors.New("unsupported scheduled command")
	ErrEntryNotFound    = errors.New("entry not found")
	ErrInvalidLine      = errors.New("invalid line number")
	ErrInvalidField     = errors.New("invalid cron field")
	ErrInvalidFieldExpr = errors.New("invalid cron field expression")
	ErrMultipleFiles    = errors.New("multiple cron files found")
)

var allowedCommands = map[string]bool{
	"/task view": true,
	"/sync":      true,
}

type Entry struct {
	Spec   string
	ChatID int64
	Cmd    string
}

type Store struct {
	path string
	mu   sync.Mutex
}

func NewStore(root string) *Store {
	return &Store{path: filepath.Join(root, FileName)}
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) List() ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listLocked()
}

func (s *Store) Add(rawSpec, rawCmd string, chatID int64) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, err := NewEntry(rawSpec, rawCmd, chatID)
	if err != nil {
		return Entry{}, err
	}

	entries, err := s.listLocked()
	if err != nil {
		return Entry{}, err
	}
	entries = append(entries, entry)
	if err := s.writeLocked(entries); err != nil {
		return Entry{}, err
	}

	return entry, nil
}

func (s *Store) Edit(line int, rawSpec, rawCmd string) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, err := NewEntry(rawSpec, rawCmd, 0)
	if err != nil {
		return Entry{}, err
	}

	entries, err := s.listLocked()
	if err != nil {
		return Entry{}, err
	}
	if line < 1 || line > len(entries) {
		return Entry{}, ErrEntryNotFound
	}

	entry.ChatID = entries[line-1].ChatID
	entries[line-1] = entry
	if err := s.writeLocked(entries); err != nil {
		return Entry{}, err
	}

	return entry, nil
}

func (s *Store) Delete(line int) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.listLocked()
	if err != nil {
		return Entry{}, err
	}
	if line < 1 || line > len(entries) {
		return Entry{}, ErrEntryNotFound
	}

	entry := entries[line-1]
	entries = append(entries[:line-1], entries[line:]...)
	if err := s.writeLocked(entries); err != nil {
		return Entry{}, err
	}

	return entry, nil
}

func (s *Store) listLocked() ([]Entry, error) {
	if err := s.ensureSingleFileLocked(); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	var entries []Entry
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		entry, err := parseStoredLine(line)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func (s *Store) writeLocked(entries []Entry) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	if err := s.ensureSingleFileLocked(); err != nil {
		return err
	}

	var lines []string
	for _, entry := range entries {
		lines = append(lines, formatStoredLine(entry))
	}

	content := ""
	if len(lines) > 0 {
		content = strings.Join(lines, "\n") + "\n"
	}
	return os.WriteFile(s.path, []byte(content), 0644)
}

func (s *Store) ensureSingleFileLocked() error {
	dir := filepath.Dir(s.path)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		files = append(files, entry.Name())
	}

	if len(files) == 0 {
		return nil
	}
	if len(files) == 1 && files[0] == filepath.Base(s.path) {
		return nil
	}
	return ErrMultipleFiles
}

func NewEntry(rawSpec, rawCmd string, chatID int64) (Entry, error) {
	spec, err := NormalizeSpec(rawSpec)
	if err != nil {
		return Entry{}, err
	}
	cmd, err := NormalizeCommand(rawCmd)
	if err != nil {
		return Entry{}, err
	}
	return Entry{Spec: spec, ChatID: chatID, Cmd: cmd}, nil
}

func NormalizeSpec(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrInvalidSpec
	}

	if t, err := time.Parse("15:04", raw); err == nil {
		return fmt.Sprintf("%d %d * * *", t.Minute(), t.Hour()), nil
	}

	fields := strings.Fields(raw)
	if len(fields) != 5 {
		return "", ErrInvalidSpec
	}

	ranges := [][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 7}}
	normalized := make([]string, len(fields))
	for i, field := range fields {
		norm, err := normalizeField(field, ranges[i][0], ranges[i][1])
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrInvalidSpec, err)
		}
		normalized[i] = norm
	}

	return strings.Join(normalized, " "), nil
}

func NormalizeCommand(raw string) (string, error) {
	cmd := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if !allowedCommands[cmd] {
		return "", ErrUnsupportedCmd
	}
	return cmd, nil
}

func Due(entries []Entry, at time.Time) ([]Entry, error) {
	var due []Entry
	for _, entry := range entries {
		ok, err := Matches(entry.Spec, at)
		if err != nil {
			return nil, err
		}
		if ok {
			due = append(due, entry)
		}
	}
	return due, nil
}

func Matches(spec string, at time.Time) (bool, error) {
	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return false, ErrInvalidSpec
	}

	values := []int{at.Minute(), at.Hour(), at.Day(), int(at.Month()), int(at.Weekday())}
	ranges := [][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 7}}
	for i, field := range fields {
		ok, err := matchField(field, values[i], ranges[i][0], ranges[i][1], i == 4)
		if err != nil {
			return false, fmt.Errorf("%w: %v", ErrInvalidSpec, err)
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func formatStoredLine(entry Entry) string {
	return fmt.Sprintf("%s\t%d\t%s", entry.Spec, entry.ChatID, entry.Cmd)
}

func parseStoredLine(line string) (Entry, error) {
	parts := strings.SplitN(line, "\t", 3)
	if len(parts) != 3 {
		return Entry{}, fmt.Errorf("%w: malformed stored entry", ErrInvalidSpec)
	}
	chatID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return Entry{}, fmt.Errorf("%w: invalid chat id", ErrInvalidSpec)
	}
	return NewEntry(parts[0], parts[2], chatID)
}

func normalizeField(field string, min, max int) (string, error) {
	if field == "" {
		return "", ErrInvalidField
	}
	parts := strings.Split(field, ",")
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return "", ErrInvalidFieldExpr
		}
		norm, err := normalizePart(part, min, max)
		if err != nil {
			return "", err
		}
		normalized = append(normalized, norm)
	}
	return strings.Join(normalized, ","), nil
}

func normalizePart(part string, min, max int) (string, error) {
	base, step := part, ""
	if strings.Contains(part, "/") {
		chunks := strings.Split(part, "/")
		if len(chunks) != 2 || chunks[0] == "" || chunks[1] == "" {
			return "", ErrInvalidFieldExpr
		}
		base, step = chunks[0], chunks[1]
		stepNum, err := strconv.Atoi(step)
		if err != nil || stepNum <= 0 {
			return "", ErrInvalidFieldExpr
		}
	}

	switch {
	case base == "*":
	case strings.Contains(base, "-"):
		bounds := strings.Split(base, "-")
		if len(bounds) != 2 {
			return "", ErrInvalidFieldExpr
		}
		start, err := parseBound(bounds[0], min, max)
		if err != nil {
			return "", err
		}
		end, err := parseBound(bounds[1], min, max)
		if err != nil {
			return "", err
		}
		if start > end {
			return "", ErrInvalidFieldExpr
		}
	default:
		if _, err := parseBound(base, min, max); err != nil {
			return "", err
		}
	}

	if step == "" {
		return base, nil
	}
	return base + "/" + step, nil
}

func parseBound(raw string, min, max int) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil || n < min || n > max {
		return 0, ErrInvalidField
	}
	return n, nil
}

func matchField(field string, value, min, max int, sundayField bool) (bool, error) {
	parts := strings.Split(field, ",")
	for _, part := range parts {
		ok, err := matchPart(part, value, min, max, sundayField)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func matchPart(part string, value, min, max int, sundayField bool) (bool, error) {
	base, step := part, 1
	if strings.Contains(part, "/") {
		chunks := strings.Split(part, "/")
		if len(chunks) != 2 {
			return false, ErrInvalidFieldExpr
		}
		base = chunks[0]
		n, err := strconv.Atoi(chunks[1])
		if err != nil || n <= 0 {
			return false, ErrInvalidFieldExpr
		}
		step = n
	}

	values, err := expandBase(base, min, max, sundayField)
	if err != nil {
		return false, err
	}
	sort.Ints(values)
	for i, candidate := range values {
		if i%step != 0 {
			continue
		}
		if candidate == value {
			return true, nil
		}
	}
	return false, nil
}

func expandBase(base string, min, max int, sundayField bool) ([]int, error) {
	switch {
	case base == "*":
		values := make([]int, 0, max-min+1)
		for i := min; i <= max; i++ {
			if sundayField && i == 7 {
				values = append(values, 0)
				continue
			}
			values = append(values, i)
		}
		return values, nil
	case strings.Contains(base, "-"):
		bounds := strings.Split(base, "-")
		if len(bounds) != 2 {
			return nil, ErrInvalidFieldExpr
		}
		start, err := parseBound(bounds[0], min, max)
		if err != nil {
			return nil, err
		}
		end, err := parseBound(bounds[1], min, max)
		if err != nil {
			return nil, err
		}
		if start > end {
			return nil, ErrInvalidFieldExpr
		}
		values := make([]int, 0, end-start+1)
		for i := start; i <= end; i++ {
			if sundayField && i == 7 {
				values = append(values, 0)
				continue
			}
			values = append(values, i)
		}
		return values, nil
	default:
		n, err := parseBound(base, min, max)
		if err != nil {
			return nil, err
		}
		if sundayField && n == 7 {
			n = 0
		}
		return []int{n}, nil
	}
}
