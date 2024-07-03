package utils

import (
	"fmt"
	"time"
)

func TimeAgo(commitTime, currentTime time.Time) string {
	duration := currentTime.Sub(commitTime)

	// Calculate total seconds
	totalSeconds := int(duration.Seconds())
	if totalSeconds < 60 {
		if totalSeconds == 0 {
			totalSeconds = 1
		}
		return fmt.Sprintf("%ds ago", totalSeconds)
	}

	// Calculate total minutes
	totalMinutes := int(duration.Minutes())
	if totalMinutes < 60 {
		if totalMinutes == 0 {
			totalMinutes = 1
		}
		return fmt.Sprintf("%dm ago", totalMinutes)
	}

	// Calculate total hours
	totalHours := int(duration.Hours())
	if totalHours < 24 {
		if totalHours == 0 {
			totalHours = 1
		}
		return fmt.Sprintf("%dh ago", totalHours)
	}

	// Calculate total days
	totalDays := int(duration.Hours() / 24)
	if totalDays == 0 {
		totalDays = 1
	}
	return fmt.Sprintf("%dd ago", totalDays)
}
