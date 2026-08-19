// Small formatters, kept out of the layout code so that reads as layout.
package shell

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	return string(b[p:])
}

func itoa64(v uint64) string { return itoa(int(v)) }

func msToS(ms uint32) string {
	s := ms / 1000
	frac := (ms % 1000) / 10
	return itoa(int(s)) + "." + pad2(int(frac)) + " s"
}

func pad2(v int) string {
	if v < 10 {
		return "0" + itoa(v)
	}
	return itoa(v)
}
