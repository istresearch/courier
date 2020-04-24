package postmaster

import (
"fmt"
"strings"
"time"
)

type ISO8601WithMilli struct {
	time.Time
}

const milliLayout = "2006-01-02T15:04:05.000Z"

func (t *ISO8601WithMilli) UnmarshalJSON(b []byte) error {
	var err error

	s := strings.Trim(string(b), "\"")

	if s == "null" || len(s) == 0 {
		t.Time = time.Time{}
		return nil
	}

	t.Time, err = time.Parse(milliLayout, s)

	return err
}

func (t *ISO8601WithMilli) MarshalJSON() ([]byte, error) {
	if t.Time.UnixNano() == (time.Time{}).UnixNano() {
		return []byte("null"), nil
	}

	return []byte(fmt.Sprintf("\"%s\"", t.Time.Format(milliLayout))), nil
}