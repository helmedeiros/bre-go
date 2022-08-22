package indexed

import "strconv"

func formatFloat(f float64) string         { return strconv.FormatFloat(f, 'g', -1, 64) }
func parseFloat(s string) (float64, error) { return strconv.ParseFloat(s, 64) }
