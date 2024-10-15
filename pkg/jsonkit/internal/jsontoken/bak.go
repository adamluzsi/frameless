package jsontoken

// type runeScanner struct {
// 	I io.Reader
// 	s *bufio.Scanner
// 	b []rune
// }

// func (s *runeScanner) init() {
// 	if s.s == nil {
// 		scr := bufio.NewScanner(s.I)
// 		scr.Split(bufio.ScanRunes)
// 		s.s = scr
// 	}
// }

// func (s *runeScanner) ReadRune() (rune, error) {
// 	if 0 < len(s.b) {
// 		if r, ok := slicekit.Shift(&s.b); ok {
// 			return r, nil
// 		}
// 		// this should not happen
// 	}
// 	if !s.s.Scan() {
// 		return 0, s.s.Err()
// 	}
// 	rs := []rune(s.s.Text())
// 	if len(rs) == 0 {
// 		// eof, probably
// 		return 0, io.EOF
// 	}
// 	if len(rs) == 1 {
// 		return rs[0], nil
// 	}
// 	r := rs[0]
// 	s.b = append(s.b, rs[1:]...)
// 	return r, nil
// }

// func (s *runeScanner) Peek() (rune, error) {
// 	if len(s.b) == 0 {
// 		if !s.s.Scan() {
// 			return 0, s.s.Err()
// 		}
// 		s.b = []rune(s.s.Text())
// 	}
// 	return s.b[0], nil
// }
