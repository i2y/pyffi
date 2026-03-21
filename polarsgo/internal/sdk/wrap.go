package polars

import "github.com/i2y/pyffi"

// WrapDataFrame wraps an existing Python DataFrame object.
func WrapDataFrame(obj *pyffi.Object, rt *pyffi.Runtime) *DataFrame {
	return &DataFrame{obj: obj, rt: rt}
}

// WrapLazyFrame wraps an existing Python LazyFrame object.
func WrapLazyFrame(obj *pyffi.Object, rt *pyffi.Runtime) *LazyFrame {
	return &LazyFrame{obj: obj, rt: rt}
}
