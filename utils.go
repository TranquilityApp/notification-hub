package hub

func containsString(target string, source []string) bool {
	ok := false
	for _, s := range source {
		if s == target {
			ok = true
			break
		}
	}
	return ok
}
