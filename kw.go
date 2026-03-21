package pyffi

// KW is a convenience type for keyword arguments.
//
//	obj.Call(data, pyffi.KW{"indent": 4})
type KW = map[string]any
