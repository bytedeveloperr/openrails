package ptr

import "time"

func False() *bool {
	b := false
	return &b
}

func True() *bool {
	b := true
	return &b
}

func Bool(b bool) *bool {
	return &b
}

func Int(i int) *int {
	return &i
}

func Int64(i int64) *int64 {
	return &i
}
func Uint(i uint) *uint {
	return &i
}

func Uint64(i uint64) *uint64 {
	return &i
}

func Float64(f float64) *float64 {
	return &f
}

func String(s string) *string {
	return &s
}

func Time(t time.Time) *time.Time {
	return &t
}
