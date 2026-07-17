//go:build !linux

package workerstatus

func systemLoad5() (float64, bool, error) {
	return 0, false, nil
}
