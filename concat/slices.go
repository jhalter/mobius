package concat

// Slices is a utility function to make appending multiple slices less painful and more efficient
// Source: https://stackoverflow.com/questions/37884361/concat-multiple-slices-in-golang
func Slices(slices ...[]byte) []byte {
	var totalLen int
	for _, s := range slices {
		totalLen += len(s)
	}
	tmp := make([]byte, totalLen)
	var i int
	for _, s := range slices {
		i += copy(tmp[i:], s)
	}

	return tmp
}
