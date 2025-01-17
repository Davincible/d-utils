package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		*d = Duration(time.Duration(value))
		return nil
	case string:
		// First try to parse the standard duration string
		tmp, err := time.ParseDuration(value)
		if err == nil {
			*d = Duration(tmp)
			fmt.Println("Returning nil parse")
			return nil
		}

		// If parsing the standard duration string fails, check for days format
		re := regexp.MustCompile(`(\d+d)?(\d+h)?(\d+m)?(\d+s)?`)
		matches := re.FindStringSubmatch(value)
		if matches == nil {
			return errors.New("invalid duration format")
		}

		var duration time.Duration

		hasMatch := false
		for _, match := range matches[1:] {
			if match == "" {
				continue
			}

			hasMatch = true

			unit := match[len(match)-1]
			num, err := strconv.Atoi(match[:len(match)-1])
			if err != nil {
				return fmt.Errorf("invalid duration segment: %v", match)
			}
			switch unit {
			case 'd':
				duration += time.Duration(num) * 24 * time.Hour
			case 'h':
				duration += time.Duration(num) * time.Hour
			case 'm':
				duration += time.Duration(num) * time.Minute
			case 's':
				duration += time.Duration(num) * time.Second
			}
		}

		if !hasMatch {
			return errors.New("invalid duration")
		}

		*d = Duration(duration)
		return nil
	default:
		return errors.New("invalid duration")
	}
}
