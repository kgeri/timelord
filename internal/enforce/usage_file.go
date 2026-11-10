package enforce

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// UsageFileName is the usage data file in a managed user's home directory.
const UsageFileName = "timelord-schedule.csv"

const dateLayout = "2006-01-02"

// readUsageFile returns the used minutes recorded for date. Old dates are
// ignored. A missing file returns fs.ErrNotExist.
func readUsageFile(path, date string) (time.Duration, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		return 0, err
	}

	var used time.Duration
	for i, row := range rows {
		if len(row) == 0 {
			continue
		}
		rowDate := strings.TrimSpace(row[0])
		if i == 0 && strings.EqualFold(rowDate, "DATE") {
			continue
		}
		if rowDate != date {
			continue
		}
		if len(row) < 2 {
			return 0, fmt.Errorf("row %d: expected DATE,USED_MINUTES", i+1)
		}

		minutes, err := strconv.Atoi(strings.TrimSpace(row[1]))
		if err != nil || minutes < 0 {
			return 0, fmt.Errorf("row %d: invalid used minutes %q", i+1, row[1])
		}
		used = time.Duration(minutes) * time.Minute
	}
	return used, nil
}

// writeUsageFile replaces path with the usage recorded for date. Used seconds
// are rounded up to the next minute so a restart cannot grant unused time.
func writeUsageFile(path, date string, used time.Duration) error {
	minutes := int((used + time.Minute - 1) / time.Minute)
	if minutes < 0 {
		minutes = 0
	}

	data := fmt.Sprintf("DATE,USED_MINUTES\n%s,%d\n", date, minutes)

	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".timelord-schedule.csv-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	if _, err := temp.WriteString(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}
