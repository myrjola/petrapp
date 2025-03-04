package ptr

// Ref returns a pointer to the value passed as argument.
//
// Might be replaced by new(T, v) in the future
// https://github.com/golang/go/issues/45624#issuecomment-2671497947
func Ref[T any](v T) *T {
	return &v
}
