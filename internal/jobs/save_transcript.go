package jobs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SaveTranscript publishes a dated Markdown file without replacing any existing note.
func SaveTranscript(dir string, date time.Time, title, id, body string) (string, error) {
	base := filepath.Join(dir, date.Format("2006-01-02"))
	for suffix := 0; ; suffix++ {
		name := title
		if suffix == 0 {
			name = strings.TrimSpace(name)
		}
		if suffix == 1 {
			name = fmt.Sprintf("%s (%s)", title, id)
		}
		if suffix > 1 {
			name = fmt.Sprintf("%s (%s-%d)", title, id, suffix)
		}
		path := base + " - " + name + ".md"
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if os.IsExist(err) {
				continue
			}
			return "", fmt.Errorf("create transcript %q: %w", path, err)
		}
		if _, err := f.WriteString(body); err != nil {
			_ = f.Close()
			_ = os.Remove(path)
			return "", fmt.Errorf("write transcript %q: %w", path, err)
		}
		if err := f.Close(); err != nil {
			return "", fmt.Errorf("close transcript %q: %w", path, err)
		}
		return path, nil
	}
}
