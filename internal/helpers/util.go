package helpers

import "fmt"

func ComputeJobName(prefixStr, nameStr, uidStr string) string {
	// extract up to 8 alphanumeric chars from uidStr, lowercased
	uidPart := make([]byte, 0, 8)
	for i := 0; i < len(uidStr) && len(uidPart) < 8; i++ {
		b := uidStr[i]
		if b >= 'A' && b <= 'Z' {
			b = b + ('a' - 'A')
		}
		if (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') {
			uidPart = append(uidPart, b)
		}
	}
	for len(uidPart) < 8 {
		uidPart = append(uidPart, '0')
	}

	// build and sanitize main part (prefix-name)
	main := prefixStr + "-" + nameStr
	out := make([]byte, 0, len(main))
	lastHyphen := false
	for i := 0; i < len(main); i++ {
		b := main[i]
		if b >= 'A' && b <= 'Z' {
			b = b + ('a' - 'A')
		}
		if (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') {
			out = append(out, b)
			lastHyphen = false
		} else {
			if !lastHyphen && len(out) > 0 {
				out = append(out, '-')
				lastHyphen = true
			}
		}
	}
	for len(out) > 0 && out[len(out)-1] == '-' {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return string(uidPart)
	}

	// ensure total length <= 63, reserve 2 hyphens and uid length
	uidLen := len(uidPart)
	maxMain := 63 - uidLen - 2
	if maxMain < 1 {
		return string(uidPart)
	}
	if len(out) > maxMain {
		out = out[:maxMain]
		for len(out) > 0 && out[len(out)-1] == '-' {
			out = out[:len(out)-1]
		}
		if len(out) == 0 {
			return string(uidPart)
		}
	}

	return fmt.Sprintf("%s-%s", string(out), string(uidPart))
}
