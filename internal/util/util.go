package util

import "fmt"

func FormatLatLng(lat, lng float64) string {
	if lat == 0 || lng == 0 {
		return ""
	}
	return fmt.Sprintf("%f,%f", lat, lng)
}
