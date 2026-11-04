package app

import "syscall"

func init() {
	sysctlMem = func() int64 {
		s, err := syscall.Sysctl("hw.memsize")
		if err != nil {
			return 0
		}
		b := []byte(s)
		for len(b) < 8 { // Sysctl trims trailing NULs
			b = append(b, 0)
		}
		var v int64
		for i := 7; i >= 0; i-- {
			v = v<<8 | int64(b[i])
		}
		return v
	}
}
